package brandcapture

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type staticResolver struct {
	addresses map[string][]net.IPAddr
}

func (resolver staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addresses, ok := resolver.addresses[host]
	if !ok {
		return nil, errors.New("unexpected host " + host)
	}
	return addresses, nil
}

func TestParsePublicURLRejectsUnsafeDestinations(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd", "https://user@example.com", "http://localhost", "http://service.local",
		"http://127.0.0.1", "http://10.0.0.1", "http://169.254.169.254", "http://192.0.2.1",
		"http://[::1]", "http://[fc00::1]", "http://[2001:db8::1]",
	} {
		if _, err := ParsePublicURL(raw); err == nil {
			t.Errorf("ParsePublicURL(%q) unexpectedly succeeded", raw)
		}
	}
	if parsed, err := ParsePublicURL("https://example.com/path#fragment"); err != nil || parsed.Fragment != "" {
		t.Fatalf("public URL parse=%v err=%v", parsed, err)
	}
}

func TestFetcherPinsValidatedDNSAndExtractsExternalStylesheet(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><meta property="og:site_name" content="Example"><link rel="stylesheet" href="/brand.css"><link rel="stylesheet" href="/second.css"></head><body><nav><a href="/products">Products</a></nav></body></html>`))
		case "/brand.css":
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(`@import url("/never.css"); body{color:#123456;font-family:Acme Sans, sans-serif}`))
		case "/second.css":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`body{color:#ffffff}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL, _ := url.Parse(server.URL)
	var mu sync.Mutex
	var dialed []string
	fetcher := &Fetcher{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"capture.example": {{IP: net.ParseIP("93.184.216.34")}},
		}},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			mu.Lock()
			dialed = append(dialed, address)
			mu.Unlock()
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		Timeout: 5 * time.Second,
	}
	candidate, err := fetcher.Capture(t.Context(), "http://capture.example:"+serverURL.Port()+"/")
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Organization != "Example" || !contains(candidate.Colors, "#123456") || !contains(candidate.Fonts, "Acme Sans, sans-serif") {
		t.Fatalf("external stylesheet was not applied: %+v", candidate)
	}
	if len(candidate.Warnings) != 1 || !strings.Contains(candidate.Warnings[0], "second.css") {
		t.Fatalf("stylesheet failure was not visible: %+v", candidate.Warnings)
	}
	if contains(requested, "/never.css") {
		t.Fatalf("CSS @import must not be fetched: %v", requested)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dialed) != 3 {
		t.Fatalf("dial count=%d addresses=%v", len(dialed), dialed)
	}
	for _, address := range dialed {
		if !strings.HasPrefix(address, "93.184.216.34:") {
			t.Fatalf("dial was not pinned to validated IP: %v", dialed)
		}
	}
}

func TestFetcherRejectsPrivateDNSAndRedirectBeforeSecondDial(t *testing.T) {
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://private.example/secret")
		w.WriteHeader(http.StatusFound)
	}))
	defer redirectServer.Close()
	serverURL, _ := url.Parse(redirectServer.URL)
	resolver := staticResolver{addresses: map[string][]net.IPAddr{
		"capture.example": {{IP: net.ParseIP("93.184.216.34")}},
		"private.example": {{IP: net.ParseIP("127.0.0.1")}},
		"mixed.example":   {{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("10.0.0.1")}},
	}}
	dials := 0
	fetcher := &Fetcher{
		Resolver: resolver,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dials++
			return (&net.Dialer{}).DialContext(ctx, network, redirectServer.Listener.Addr().String())
		},
		Timeout: 5 * time.Second,
	}
	if _, err := fetcher.Capture(t.Context(), "http://mixed.example/"); !errors.Is(err, ErrNonPublicDestination) || dials != 0 {
		t.Fatalf("mixed DNS err=%v dials=%d", err, dials)
	}
	_, err := fetcher.Capture(t.Context(), "http://capture.example:"+serverURL.Port()+"/")
	if !errors.Is(err, ErrNonPublicDestination) || dials != 1 {
		t.Fatalf("private redirect err=%v dials=%d", err, dials)
	}
}

func TestFetcherEnforcesMediaTypeAndDecodedSize(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"media": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		},
		"size": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(strings.Repeat("x", int(MaxHTMLBytes)+1)))
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			serverURL, _ := url.Parse(server.URL)
			fetcher := &Fetcher{
				Resolver: staticResolver{addresses: map[string][]net.IPAddr{"capture.example": {{IP: net.ParseIP("93.184.216.34")}}}},
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
				},
				Timeout: 5 * time.Second,
			}
			if _, err := fetcher.Capture(t.Context(), "http://capture.example:"+serverURL.Port()+"/"); err == nil {
				t.Fatal("unsafe response unexpectedly succeeded")
			}
		})
	}
}

func TestFetcherClassifiesSourceAccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	serverURL, _ := url.Parse(server.URL)
	fetcher := &Fetcher{
		Resolver: staticResolver{addresses: map[string][]net.IPAddr{
			"capture.example": {{IP: net.ParseIP("93.184.216.34")}},
		}},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
		Timeout: 5 * time.Second,
	}

	_, err := fetcher.Capture(t.Context(), "http://capture.example:"+serverURL.Port()+"/")
	if !errors.Is(err, ErrSourceAccessDenied) {
		t.Fatalf("expected access-denied classification, got %v", err)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
