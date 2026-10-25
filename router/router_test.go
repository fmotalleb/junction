package router

import (
	"net"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/fmotalleb/junction/config"
)

func TestRegistryPrepareKeepsTaggedRouteOrderAndIsolation(t *testing.T) {
	tag := "edge"
	entries := []config.EntryPoint{
		{Routing: config.RouterSNI, Listen: mustAddrPort(t, "127.0.0.1:8443"), Tag: &tag, Target: "443"},
		{Routing: config.RouterSNI, Listen: mustAddrPort(t, "127.0.0.1:8443"), Tag: &tag, Target: "9443"},
	}
	first, second := NewRegistry(), NewRegistry()
	if err := first.Prepare(entries); err != nil {
		t.Fatal(err)
	}
	if err := second.Prepare(entries[:1]); err != nil {
		t.Fatal(err)
	}
	group := first.Group(tag)
	if len(group) != 2 || group[0].Target != "443" || group[1].Target != "9443" {
		t.Fatalf("first registry group order = %#v", group)
	}
	if got := second.Group(tag); len(got) != 1 || got[0].Target != "443" {
		t.Fatalf("second registry inherited routes from first: %#v", got)
	}
	firstRoute, ok := first.Route(0)
	if !ok || firstRoute.chain == nil {
		t.Fatal("route proxy chain was not compiled")
	}
	secondRoute, ok := second.Route(0)
	if !ok || secondRoute.chain == nil || secondRoute.chain == firstRoute.chain {
		t.Fatal("registries must own separate compiled routes")
	}
	if !first.primary[0] || first.primary[1] {
		t.Fatalf("unexpected listener owners: %#v", first.primary)
	}
}

func TestRegistryPrepareRejectsIncompatibleTaggedListeners(t *testing.T) {
	tag := "shared"
	for name, entries := range map[string][]config.EntryPoint{
		"listen": {
			{Routing: config.RouterHTTPHeader, Listen: mustAddrPort(t, "127.0.0.1:8080"), Tag: &tag},
			{Routing: config.RouterHTTPHeader, Listen: mustAddrPort(t, "127.0.0.1:8081"), Tag: &tag},
		},
		"routing": {
			{Routing: config.RouterHTTPHeader, Listen: mustAddrPort(t, "127.0.0.1:8080"), Tag: &tag},
			{Routing: config.RouterSNI, Listen: mustAddrPort(t, "127.0.0.1:8080"), Tag: &tag},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := NewRegistry().Prepare(entries); err == nil {
				t.Fatal("Prepare succeeded for an incompatible group")
			}
		})
	}
}

func TestRegistryPrepareRejectsUnsupportedProxyBeforeServing(t *testing.T) {
	proxyURL, err := url.Parse("https://proxy.example:443")
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := registry.Prepare([]config.EntryPoint{{Proxy: []*url.URL{proxyURL}}}); err == nil {
		t.Fatal("Prepare accepted an unsupported proxy scheme")
	}
}

func TestValidateEntryPointBeforeServing(t *testing.T) {
	listen := mustAddrPort(t, "127.0.0.1:8080")
	for _, entry := range []config.EntryPoint{
		{Routing: config.RouterSNI, Listen: listen},
		{Routing: config.RouterHTTPHeader, Listen: listen, Target: "8081"},
		{Routing: config.RouterHTTPToHTTPS, Listen: listen, Target: "https://backend.example"},
		{Routing: config.RouterTCPRaw, Listen: listen, Target: "127.0.0.1:9000"},
		{Routing: config.RouterUDPRaw, Listen: listen, Target: "127.0.0.1:9000"},
	} {
		if err := validateEntryPoint(entry); err != nil {
			t.Errorf("valid entry %q rejected: %v", entry.Routing, err)
		}
	}
	for _, entry := range []config.EntryPoint{
		{Routing: config.RouterSNI, Target: "443"},
		{Routing: config.Router("unknown"), Listen: listen},
		{Routing: config.RouterTCPRaw, Listen: listen, Target: "not-a-host-port"},
		{Routing: config.RouterHTTPHeader, Listen: listen, Target: "70000"},
		{Routing: config.RouterSNI, Listen: listen, Timeout: -time.Second},
	} {
		if err := validateEntryPoint(entry); err == nil {
			t.Errorf("invalid entry %q unexpectedly accepted", entry.Routing)
		}
	}
}

func TestLimitedListenerReleasesPermitWhenConnectionCloses(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newLimitedListener(base, 1)
	defer listener.Close()

	client1, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client1.Close()
	server1, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server1.Close()

	client2, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client2.Close()
	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	if err := server1.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	case err := <-acceptErr:
		t.Fatalf("second connection was not accepted after permit release: %v", err)
	case <-time.After(time.Second):
		t.Fatal("connection limit permit was not released")
	}
}

func TestSourceLimiterCapsEachSourceAndReleasesSlots(t *testing.T) {
	limiter := newSourceLimiter(2)
	first, ok := limiter.acquire(rawAddr("192.0.2.1:1000"))
	if !ok {
		t.Fatal("first source slot rejected")
	}
	second, ok := limiter.acquire(rawAddr("192.0.2.1:1001"))
	if !ok {
		t.Fatal("second source slot rejected")
	}
	if _, ok := limiter.acquire(rawAddr("192.0.2.1:1002")); ok {
		t.Fatal("source exceeded its limit")
	}
	other, ok := limiter.acquire(rawAddr("192.0.2.2:1000"))
	if !ok {
		t.Fatal("a different source should have an independent limit")
	}
	first()
	third, ok := limiter.acquire(rawAddr("192.0.2.1:1002"))
	if !ok {
		t.Fatal("released source slot was not reusable")
	}
	third()
	second()
	other()
}

func mustAddrPort(t *testing.T, value string) (addr netip.AddrPort) {
	t.Helper()
	addr, err := netip.ParseAddrPort(value)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
