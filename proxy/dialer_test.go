package proxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"
)

// A proxy that accepts the socket but never answers must not park the dialer:
// every such dial held a goroutine and two sockets for the lifetime of the
// process.
func TestChainDialHonorsContextOnStalledProxy(t *testing.T) {
	lc := net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		var held []net.Conn
		defer func() {
			for _, c := range held {
				_ = c.Close()
			}
		}()
		for {
			conn, acceptErr := l.Accept()
			if acceptErr != nil {
				return
			}
			held = append(held, conn)
		}
	}()

	chain, err := NewChain([]*url.URL{{
		Scheme: "socks5",
		Host:   l.Addr().String(),
	}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, dialErr := chain.DialContext(ctx, "tcp", "192.0.2.1:80")
		done <- dialErr
	}()

	select {
	case dialErr := <-done:
		if dialErr == nil {
			t.Fatal("expected dial to fail against a stalled proxy")
		}
		if !errors.Is(dialErr, context.DeadlineExceeded) && !errors.Is(dialErr, context.Canceled) {
			t.Logf("dial failed as expected with: %v", dialErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dial ignored the context deadline and is still blocked")
	}
}
