package proxy

import (
	"context"
	"net/netip"
	"net/url"
	"testing"
)

func TestDestinationPolicyAllowAndBlockRules(t *testing.T) {
	policy, err := parseDestinationPolicy([]string{"10.0.0.0/8"}, []string{"10.1.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}
	for value, want := range map[string]bool{
		"10.2.3.4":        true,
		"10.1.2.3":        false,
		"192.0.2.1":       false,
		"::ffff:10.2.3.4": true,
	} {
		if got := policy.allows(netip.MustParseAddr(value)); got != want {
			t.Errorf("allows(%s) = %t, want %t", value, got, want)
		}
	}
}

func TestDestinationPolicyRejectsUnverifiableProxyChains(t *testing.T) {
	proxyURL, err := url.Parse("socks5://proxy.example:1080")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewChainWithDestinationPolicy([]*url.URL{proxyURL}, nil, []string{"127.0.0.0/8"}); err == nil {
		t.Fatal("destination policy was accepted with a remote-resolving proxy chain")
	}
}

func TestDestinationPolicyBlocksIPBeforeDial(t *testing.T) {
	chain, err := NewChainWithDestinationPolicy(nil, nil, []string{"127.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chain.DialContext(context.Background(), "tcp", "127.0.0.1:1"); err == nil {
		t.Fatal("loopback target was not blocked")
	}
}

func TestDestinationPolicyRejectsInvalidCIDR(t *testing.T) {
	if _, err := NewChainWithDestinationPolicy(nil, []string{"not-a-cidr"}, nil); err == nil {
		t.Fatal("invalid destination CIDR was accepted")
	}
}
