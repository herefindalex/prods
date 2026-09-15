package webapp

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyClientSourceWalksChainFromTransportBoundary(t *testing.T) {
	trust, err := newProxyTrust([]string{"10.0.0.0/8", "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		remote     string
		forwarded  string
		wantSource string
	}{
		{name: "untrusted transport ignores spoofed header", remote: "203.0.113.9:443", forwarded: "198.51.100.7", wantSource: "203.0.113.9"},
		{name: "trusted edge accepts client", remote: "10.0.0.9:443", forwarded: "198.51.100.7", wantSource: "198.51.100.7"},
		{name: "rightmost untrusted hop is boundary", remote: "10.0.0.9:443", forwarded: "198.51.100.7, 192.0.2.4, 10.0.0.8", wantSource: "192.0.2.4"},
		{name: "malformed chain falls back direct", remote: "10.0.0.9:443", forwarded: "198.51.100.7, unknown", wantSource: "10.0.0.9"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://catalog.example.test/search", nil)
			request.RemoteAddr = test.remote
			request.Header.Set("X-Forwarded-For", test.forwarded)
			if source := trust.clientSource(request); source != test.wantSource {
				t.Fatalf("source=%q want=%q", source, test.wantSource)
			}
		})
	}
}

func TestTrustedProxyConfigurationRejectsInvalidOrEmptyEntries(t *testing.T) {
	for _, values := range [][]string{{""}, {"not-an-address"}, {"10.0.0.0/99"}} {
		if _, err := newProxyTrust(values); err == nil {
			t.Fatalf("trusted proxy values accepted: %v", values)
		}
	}
	trust, err := newProxyTrust(nil)
	if err != nil || trust.configured() {
		t.Fatalf("empty trusted proxy policy configured=%v err=%v", trust.configured(), err)
	}
}
