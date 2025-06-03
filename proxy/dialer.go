package proxy

import "golang.org/x/net/proxy"

func NewDialer(addr string) (proxy.Dialer, error) {
	if addr == "direct" {
		return proxy.Direct, nil
	}
	return proxy.SOCKS5("tcp", addr, nil, proxy.Direct)
}
