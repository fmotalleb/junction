package proxy

import (
	"errors"
	"strings"

	"golang.org/x/net/proxy"
)

// NewDialer constructs a chain of SOCKS5 proxies given a comma-separated list.
// e.g.: "proxy1:1080,proxy2:1080,proxy3:1080".
func NewDialer(chain string) (proxy.Dialer, error) {
	chain = strings.TrimSpace(chain)
	if chain == "" || chain == "direct" {
		return proxy.Direct, nil
	}

	parts := strings.Split(chain, ",")
	var dialer proxy.Dialer = proxy.Direct

	// Build the proxy chain from last to first
	for i := len(parts) - 1; i >= 0; i-- {
		addr := strings.TrimSpace(parts[i])
		if addr == "" {
			return nil, errors.New("invalid empty proxy address in chain")
		}
		socks5Dialer, err := proxy.SOCKS5("tcp", addr, nil, dialer)
		if err != nil {
			return nil, err
		}
		dialer = socks5Dialer
	}

	return dialer, nil
}
