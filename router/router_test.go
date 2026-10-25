package router

import (
	"net/netip"
	"net/url"
	"testing"

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

func mustAddrPort(t *testing.T, value string) (addr netip.AddrPort) {
	t.Helper()
	addr, err := netip.ParseAddrPort(value)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
