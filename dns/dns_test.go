package dns

import (
	"context"
	"net"
	"net/netip"
	"reflect"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/fmotalleb/go-tools/matcher"

	"github.com/fmotalleb/junction/config"
)

const exampleDomain = "example.com"

func mustMatcher(t *testing.T, pattern string) *matcher.Matcher {
	t.Helper()
	m := new(matcher.Matcher)
	out, err := m.Decode(reflect.TypeOf(""), pattern)
	if err != nil {
		t.Fatalf("compile matcher %q: %v", pattern, err)
	}
	return out.(*matcher.Matcher)
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse cidr %q: %v", s, err)
	}
	return n
}

func ip(s string) *net.IP {
	parsed := net.ParseIP(s)
	return &parsed
}

type plainAddr string

func (a plainAddr) Network() string { return "udp" }
func (a plainAddr) String() string  { return string(a) }

func TestNewResolversValidation(t *testing.T) {
	t.Run("missing answer IP is rejected", func(t *testing.T) {
		_, err := newResolvers([]*config.DNSResult{{From: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}}})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no result IP configured")
	})

	t.Run("valid entries keep their answer", func(t *testing.T) {
		resolvers, err := newResolvers([]*config.DNSResult{
			{From: []*net.IPNet{mustCIDR(t, "192.168.1.0/24")}, Result: ip("10.10.10.10")},
			{From: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}, Result: ip("10.20.20.20")},
		})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(resolvers))
		assert.Equal(t, "10.10.10.10", resolvers[0].answer.String())
		assert.Equal(t, "10.20.20.20", resolvers[1].answer.String())
	})
}

func TestFindAnswer(t *testing.T) {
	resolvers, err := newResolvers([]*config.DNSResult{
		{From: []*net.IPNet{mustCIDR(t, "192.168.1.0/24")}, Result: ip("10.10.10.10")},
		{From: []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}, Result: ip("10.20.20.20")},
	})
	assert.NoError(t, err)
	h := newHandler(context.Background(), resolvers, "", nil)

	assert.Equal(t, "10.10.10.10", h.findAnswer(&net.UDPAddr{IP: net.ParseIP("192.168.1.7"), Port: 53}).String())
	assert.Equal(t, "10.20.20.20", h.findAnswer(&net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 53}).String())
	assert.Zero(t, h.findAnswer(&net.UDPAddr{IP: net.ParseIP("172.16.0.1"), Port: 53}))
	assert.Zero(t, h.findAnswer(plainAddr("no-port-here")))
}

func TestIsAllowed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		allowList []string
		question  string
		want      bool
	}{
		{"no allow list allows everything", nil, "anything.example.org.", true},
		{"trailing dot is ignored", []string{exampleDomain}, "example.com.", true},
		{"exact name is allowed", []string{exampleDomain}, exampleDomain, true},
		{"other name is refused", []string{exampleDomain}, "other.com.", false},
		{"wildcard matches subdomains", []string{"*.example.com"}, "api.example.com.", true},
		{"wildcard does not match base domain", []string{"*.example.com"}, "example.com.", false},
		{"any pattern in the list is enough", []string{"a.com", "b.com"}, "b.com.", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var allowList []matcher.Matcher
			for _, p := range tc.allowList {
				allowList = append(allowList, *mustMatcher(t, p))
			}
			h := newHandler(context.Background(), nil, "", allowList)
			assert.Equal(t, tc.want, h.IsAllowed(tc.question))
		})
	}
}

func TestServeValidatesConfig(t *testing.T) {
	assert.Error(t, Serve(context.Background(), config.FakeDNS{}))

	listen := netip.MustParseAddrPort("127.0.0.1:0")
	cfg := config.FakeDNS{
		Listen:     &listen,
		ReturnAddr: []*config.DNSResult{{From: []*net.IPNet{mustCIDR(t, "0.0.0.0/0")}}},
	}
	assert.Error(t, Serve(context.Background(), cfg), "an answer entry without a result IP must be rejected before binding")
}

// TestServeStopsOnCancel guards the shutdown path: the server used to block in
// dns.ActivateAndServe forever because the listener was never closed when the
// context was canceled, which left server.Serve waiting on its wait group.
func TestServeStopsOnCancel(t *testing.T) {
	listen := netip.MustParseAddrPort("127.0.0.1:0")
	cfg := config.FakeDNS{
		Listen: &listen,
		ReturnAddr: []*config.DNSResult{
			{From: []*net.IPNet{mustCIDR(t, "0.0.0.0/0")}, Result: ip("10.10.10.10")},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg) }()

	select {
	case err := <-done:
		t.Fatalf("dns.Serve returned before it was asked to stop: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("dns.Serve did not stop after the context was canceled")
	}
}
