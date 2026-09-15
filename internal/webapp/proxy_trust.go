package webapp

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type proxyTrust struct {
	prefixes []netip.Prefix
}

func newProxyTrust(values []string) (proxyTrust, error) {
	trust := proxyTrust{prefixes: make([]netip.Prefix, 0, len(values))}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return proxyTrust{}, fmt.Errorf("trusted proxy entry is empty")
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil {
				return proxyTrust{}, fmt.Errorf("invalid trusted proxy %q", value)
			}
			address = address.Unmap()
			prefix = netip.PrefixFrom(address, address.BitLen())
		} else {
			prefix = prefix.Masked()
		}
		trust.prefixes = append(trust.prefixes, prefix)
	}
	return trust, nil
}

func (trust proxyTrust) configured() bool {
	return len(trust.prefixes) != 0
}

func (trust proxyTrust) clientSource(request *http.Request) string {
	direct, ok := requestAddress(request.RemoteAddr)
	if !ok {
		return "unknown-direct-source"
	}
	if !trust.contains(direct) {
		return direct.String()
	}

	values := request.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return direct.String()
	}
	var hops []string
	for _, value := range values {
		hops = append(hops, strings.Split(value, ",")...)
	}
	candidate := direct
	for index := len(hops) - 1; index >= 0; index-- {
		address, err := netip.ParseAddr(strings.TrimSpace(hops[index]))
		if err != nil {
			// A malformed chain cannot be partially trusted for rate-limit
			// identity. Fall back to the transport peer.
			return direct.String()
		}
		candidate = address.Unmap()
		if !trust.contains(candidate) {
			return candidate.String()
		}
	}
	return candidate.String()
}

func (trust proxyTrust) contains(address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range trust.prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func requestAddress(remote string) (netip.Addr, bool) {
	value := strings.TrimSpace(remote)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	address, err := netip.ParseAddr(strings.Trim(value, "[]"))
	if err != nil {
		return netip.Addr{}, false
	}
	return address.Unmap(), true
}
