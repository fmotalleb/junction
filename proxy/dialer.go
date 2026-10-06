package proxy

import (
	"context"
	"net"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// DefaultDialTimeout bounds a single dial attempt, including the proxy
// handshake, so a stalled upstream can not pin a goroutine and two sockets
// until the process is restarted.
const DefaultDialTimeout = 30 * time.Second

type generator func(*url.URL, proxy.Dialer) (proxy.Dialer, error)

var generators []generator

func registerGenerator(g generator) {
	generators = append(generators, g)
}

// NewDialer constructs a chain of proxy dialers from a slice of URLs.
// Supported schemes include socks5:// and ssh://.
// The chain is built in order, with each proxy connecting through the previous one.
func NewDialer(chain []*url.URL) (proxy.Dialer, error) {
	var dialer proxy.Dialer = proxy.Direct
	// Build the proxy chain from last to first
	for _, addr := range chain {
		d, err := generateDialer(*addr, dialer)
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

// Chain is a reusable, context aware front end for a proxy chain. Build it once
// per entry point instead of per connection: SSH entries re-read their key file
// on every dial otherwise.
type Chain struct {
	dialer proxy.Dialer
}

// Chain stays usable anywhere a plain proxy.Dialer is expected.
var _ proxy.Dialer = (*Chain)(nil)

// NewChain builds the dialer chain for the given proxy URLs.
func NewChain(chain []*url.URL) (*Chain, error) {
	d, err := NewDialer(chain)
	if err != nil {
		return nil, err
	}
	return &Chain{dialer: d}, nil
}

// DialContext connects to address, honoring ctx for the whole attempt
// including the proxy handshake.
func (c *Chain) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return DialWith(ctx, c.dialer, network, address)
}

// Dial connects to address without a deadline. Prefer DialContext.
func (c *Chain) Dial(network, address string) (net.Conn, error) {
	return c.DialContext(context.Background(), network, address)
}

// DialWith connects to address through an already built dialer, honoring ctx for
// the whole attempt when the dialer supports it.
func DialWith(ctx context.Context, d proxy.Dialer, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cd, ok := d.(proxy.ContextDialer); ok {
		return cd.DialContext(ctx, network, address)
	}
	return dialContext(ctx, d, network, address)
}

// dialContext runs dialers that do not implement proxy.ContextDialer in a
// goroutine. Such a dial can only be abandoned, not interrupted, so the helper
// closes the connection if it shows up after ctx was canceled.
func dialContext(ctx context.Context, d proxy.Dialer, network, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		c, err := d.Dial(network, address)
		done <- result{c, err}
	}()

	select {
	case <-ctx.Done():
		go func() {
			if r := <-done; r.conn != nil {
				_ = r.conn.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-done:
		if ctx.Err() != nil {
			if r.conn != nil {
				_ = r.conn.Close()
			}
			return nil, ctx.Err()
		}
		return r.conn, r.err
	}
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
