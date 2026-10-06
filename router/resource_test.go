package router

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fmotalleb/junction/config"
)

// Helpers and a relay test guarding against the resource leaks that made a
// long running process climb to hundreds of megabytes until the socket buffers
// were exhausted: tunnels that outlived their peers.

func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func dialTCP(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func freePort(t *testing.T) int {
	t.Helper()
	l := listenTCP(t)
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

func testEntry(t *testing.T, routing, listen, target string) config.EntryPoint {
	t.Helper()
	ap, err := netip.ParseAddrPort(listen)
	if err != nil {
		t.Fatal(err)
	}
	var r config.Router
	if err := r.Set(routing); err != nil {
		t.Fatal(err)
	}
	return config.EntryPoint{
		Routing: r,
		Listen:  ap,
		Target:  target,
		Timeout: 30 * time.Second,
	}
}

// startTCPTarget accepts connections, drains them and keeps them open until it
// is stopped, so the relay has something that stays silent. It reports how many
// connections are still open on the target side, which is the resource a leaked
// relay would keep pinned.
func startTCPTarget(t *testing.T) (addr string, open func() int, stop func()) {
	t.Helper()
	l := listenTCP(t)
	done := make(chan struct{})
	var openConns atomic.Int64
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			openConns.Add(1)
			go func(c net.Conn) {
				defer func() {
					openConns.Add(-1)
					_ = c.Close()
				}()
				buf := make([]byte, 4096)
				for {
					select {
					case <-done:
						return
					default:
					}
					_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
					if _, err := c.Read(buf); err != nil {
						var ne net.Error
						if errors.As(err, &ne) && ne.Timeout() {
							continue
						}
						return
					}
				}
			}(c)
		}
	}()
	return l.Addr().String(),
		func() int { return int(openConns.Load()) },
		func() { close(done); _ = l.Close() }
}

// waitFor polls cond until it holds or the deadline expires.
func waitFor(t *testing.T, within time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return cond()
}

func waitGoroutines(t *testing.T, base, tolerance int, within time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(within)
	current := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		runtime.GC()
		current = runtime.NumGoroutine()
		if current <= base+tolerance {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return current
}

func dumpGoroutines() string {
	buf := make([]byte, 1<<21)
	return string(buf[:runtime.Stack(buf, true)])
}

func TestTCPRawReleasesConnections(t *testing.T) {
	target, targetOpen, stop := startTCPTarget(t)
	defer stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyPort := freePort(t)
	entry := testEntry(t, "tcp-raw", fmt.Sprintf("127.0.0.1:%d", proxyPort), target)
	go func() {
		_, _ = tcpRouter(ctx, entry)
	}()
	time.Sleep(300 * time.Millisecond)

	addr := fmt.Sprintf("127.0.0.1:%d", proxyPort)
	base := runtime.NumGoroutine()

	const n = 60
	for i := range n {
		c := dialTCP(t, addr)
		_, _ = c.Write([]byte("hello"))
		switch i % 3 {
		case 0:
			_ = c.Close()
		case 1: // abrupt reset
			_ = c.(*net.TCPConn).SetLinger(0)
			_ = c.Close()
		case 2:
			time.Sleep(2 * time.Millisecond)
			_ = c.Close()
		}
	}

	var idle []net.Conn
	for range 10 {
		idle = append(idle, dialTCP(t, addr))
	}
	time.Sleep(300 * time.Millisecond)
	for _, c := range idle {
		_ = c.Close()
	}

	// The resource that matters: every connection the relay opened towards the
	// target must be gone once the peers hung up.
	if !waitFor(t, 5*time.Second, func() bool { return targetOpen() == 0 }) {
		t.Errorf("tcp-raw left %d connection(s) open on the target after the peers closed\n%s",
			targetOpen(), dumpGoroutines())
	}
	if after := waitGoroutines(t, base, 5, 5*time.Second); after > base+10 {
		t.Logf("goroutine count (diagnostic): base=%d after=%d\n%s", base, after, dumpGoroutines())
	}
}
