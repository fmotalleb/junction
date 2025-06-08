package proxy

import (
	"net/url"

	"golang.org/x/net/proxy"
)

type generator func(*url.URL, proxy.Dialer) (proxy.Dialer, error)

var generators []generator

func registerGenerator(g generator) {
	generators = append(generators, g)
}

// NewDialer constructs a chain of proxy dialers from a slice of URLs.
// Supported schemes include socks5:// and ssh://.
// The chain is built in order, with each proxy connecting through the previous one.
func NewDialer(chain []url.URL) (proxy.Dialer, error) {
	var dialer proxy.Dialer = proxy.Direct
	// Build the proxy chain from last to first
	for _, addr := range chain {
		d, err := generateDialer(addr, dialer)
		if err != nil {
			return nil, err
		}
		if d != nil {
			dialer = d
			continue
		}
	}

	return dialer, nil
}

func generateDialer(addr url.URL, dialer proxy.Dialer) (proxy.Dialer, error) {
	for _, g := range generators {
		d, err := g(&addr, dialer)
		if err != nil {
			return nil, err
		}
		if d != nil {
			return d, nil
		}
	}
	return nil, nil
}
