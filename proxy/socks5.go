package proxy

import (
	"net/url"

	"golang.org/x/net/proxy"
)

func init() {
	registerGenerator(socks5Dialer)
}

func socks5Dialer(url *url.URL, dialer proxy.Dialer) (proxy.Dialer, error) {
	if url.Scheme != "socks5" && url.Scheme != "socks5h" {
		return nil, nil
	}
	return proxy.FromURL(url, dialer)
}
