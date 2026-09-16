package brandcapture

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultTimeout       = 12 * time.Second
	DefaultMaxRedirects  = 5
	MaxResponseHeadBytes = 64 << 10
)

var (
	ErrInvalidURL           = errors.New("capture URL is invalid")
	ErrNonPublicDestination = errors.New("capture destination is not public")
	ErrSourceAccessDenied   = errors.New("capture source denied automated access")
)

type Resolver interface {
	LookupIPAddr(context.Context, string) ([]net.IPAddr, error)
}

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

// Fetcher performs a bounded, DNS-rebinding-resistant fetch. It extracts only
// a candidate and never stores source HTML, CSS, or downloaded assets.
type Fetcher struct {
	Resolver     Resolver
	DialContext  DialContextFunc
	Timeout      time.Duration
	MaxRedirects int
}

func NewFetcher() *Fetcher {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}
	return &Fetcher{
		Resolver:     net.DefaultResolver,
		DialContext:  dialer.DialContext,
		Timeout:      DefaultTimeout,
		MaxRedirects: DefaultMaxRedirects,
	}
}

func ParsePublicURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: an absolute HTTP or HTTPS URL is required", ErrInvalidURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: only HTTP and HTTPS are allowed", ErrInvalidURL)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("%w: user information is not allowed", ErrInvalidURL)
	}
	if parsed.Fragment != "" {
		parsed.Fragment = ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return nil, fmt.Errorf("%w: local host names are not allowed", ErrNonPublicDestination)
	}
	if ip := net.ParseIP(host); ip != nil && !isPublicIP(ip) {
		return nil, ErrNonPublicDestination
	}
	return parsed, nil
}

func (fetcher *Fetcher) Capture(ctx context.Context, rawURL string) (Candidate, error) {
	if fetcher == nil {
		return Candidate{}, errors.New("brand capture fetcher is not configured")
	}
	timeout := fetcher.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pageURL, body, err := fetcher.fetch(ctx, rawURL, []string{"text/html", "application/xhtml+xml"}, MaxHTMLBytes)
	if err != nil {
		return Candidate{}, err
	}
	candidate, err := Capture(pageURL.String(), strings.NewReader(string(body)))
	if err != nil {
		return Candidate{}, err
	}

	stylesheetURLs := append([]string(nil), candidate.StylesheetURLs...)
	for _, stylesheetURL := range stylesheetURLs {
		_, stylesheet, fetchErr := fetcher.fetch(ctx, stylesheetURL, []string{"text/css"}, MaxStylesheetBytes)
		if fetchErr != nil {
			candidate.Warnings = appendUnique(candidate.Warnings, "Stylesheet unavailable: "+stylesheetURL, maxCandidateValues)
			continue
		}
		ApplyStyles(&candidate, string(stylesheet))
	}
	Finalize(&candidate)
	return candidate, nil
}

func (fetcher *Fetcher) fetch(ctx context.Context, rawURL string, allowedMediaTypes []string, maximum int64) (*url.URL, []byte, error) {
	current, err := ParsePublicURL(rawURL)
	if err != nil {
		return nil, nil, err
	}
	maxRedirects := fetcher.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = DefaultMaxRedirects
	}
	for redirects := 0; ; redirects++ {
		ips, err := fetcher.resolvePublic(ctx, current.Hostname())
		if err != nil {
			return nil, nil, err
		}
		response, err := fetcher.requestPinned(ctx, current, ips)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch %s: %w", current.Redacted(), err)
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			location := response.Header.Get("Location")
			response.Body.Close()
			if location == "" {
				return nil, nil, errors.New("redirect response has no Location header")
			}
			if redirects >= maxRedirects {
				return nil, nil, errors.New("capture redirect limit exceeded")
			}
			reference, parseErr := url.Parse(location)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("invalid redirect Location: %w", parseErr)
			}
			current, err = ParsePublicURL(current.ResolveReference(reference).String())
			if err != nil {
				return nil, nil, fmt.Errorf("redirect rejected: %w", err)
			}
			// Resolve the redirect target before another request is attempted. The
			// next loop resolves again for the pinned transport and rejects any
			// changed answer that contains a non-public address.
			if _, err := fetcher.resolvePublic(ctx, current.Hostname()); err != nil {
				return nil, nil, fmt.Errorf("redirect rejected: %w", err)
			}
			continue
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
				return nil, nil, fmt.Errorf("%w: HTTP %d", ErrSourceAccessDenied, response.StatusCode)
			}
			return nil, nil, fmt.Errorf("capture source returned HTTP %d", response.StatusCode)
		}
		mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || !containsMediaType(allowedMediaTypes, mediaType) {
			return nil, nil, fmt.Errorf("capture source has unsupported Content-Type %q", response.Header.Get("Content-Type"))
		}
		if response.ContentLength > maximum {
			return nil, nil, errors.New("capture response exceeds size limit")
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
		if err != nil {
			return nil, nil, err
		}
		if int64(len(body)) > maximum {
			return nil, nil, errors.New("capture response exceeds size limit")
		}
		return current, body, nil
	}
}

func (fetcher *Fetcher) resolvePublic(ctx context.Context, host string) ([]net.IP, error) {
	resolver := fetcher.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve capture destination: %w", err)
	}
	if len(addresses) == 0 {
		return nil, errors.New("capture destination has no IP address")
	}
	ips := make([]net.IP, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return nil, ErrNonPublicDestination
		}
		key := address.IP.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ips = append(ips, append(net.IP(nil), address.IP...))
	}
	return ips, nil
}

func (fetcher *Fetcher) requestPinned(ctx context.Context, target *url.URL, ips []net.IP) (*http.Response, error) {
	baseDial := fetcher.DialContext
	if baseDial == nil {
		baseDial = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 15 * time.Second}).DialContext
	}
	wantedHost := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	transport := &http.Transport{
		Proxy:                  nil,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: MaxResponseHeadBytes,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  6 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil || strings.TrimSuffix(strings.ToLower(host), ".") != wantedHost {
				return nil, errors.New("transport attempted an unvalidated destination")
			}
			var lastErr error
			for _, ip := range ips {
				connection, dialErr := baseDial(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				lastErr = dialErr
			}
			return nil, lastErr
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml,text/css;q=0.8")
	request.Header.Set("User-Agent", "Prods-Brand-Capture/1")
	return client.Do(request)
}

func containsMediaType(allowed []string, actual string) bool {
	for _, value := range allowed {
		if strings.EqualFold(value, actual) {
			return true
		}
	}
	return false
}

func isPublicIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	for _, prefix := range nonPublicPrefixes(address.Is4()) {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func nonPublicPrefixes(ipv4 bool) []netip.Prefix {
	values := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.168.0.0/16", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	}
	if !ipv4 {
		values = []string{"::/128", "::1/128", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "fc00::/7", "fe80::/10", "ff00::/8"}
	}
	result := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		result = append(result, netip.MustParsePrefix(value))
	}
	return result
}
