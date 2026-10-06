package router

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// startHTTPTarget deliberately keeps idle connections open: an origin that
// never hangs up is exactly the case a leaked transport can not recover from.
// It reports how many connections were accepted, which is what a shared,
// pooled transport should keep low no matter how many requests are made.
func startHTTPTarget(t *testing.T) (addr string, accepted func() int, stop func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	l := listenTCP(t)
	var conns atomic.Int64
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ConnState: func(_ net.Conn, s http.ConnState) {
			if s == http.StateNew {
				conns.Add(1)
			}
		},
	}
	go func() { _ = srv.Serve(l) }()
	return l.Addr().String(),
		func() int { return int(conns.Load()) },
		func() { _ = srv.Close() }
}

func TestHTTPHeaderReusesTransport(t *testing.T) {
	target, targetAccepted, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyPort := freePort(t)
	entry := testEntry(t, "http-header", fmt.Sprintf("127.0.0.1:%d", proxyPort), "")
	entry.Features = []string{"flexible-port"}
	go func() {
		_, _ = httpHandler(ctx, entry)
	}()
	time.Sleep(300 * time.Millisecond)

	targetHost, targetPort, _ := net.SplitHostPort(target)
	client := &http.Client{Timeout: 10 * time.Second}
	base := runtime.NumGoroutine()

	const n = 30
	for i := range n {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/hello", proxyPort), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = targetHost
		req.Header.Set("Junction-Port", targetPort)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	client.CloseIdleConnections()

	// The resource that matters: a per-request transport would open one
	// upstream connection per request, a pooled one opens a handful at most.
	if got := targetAccepted(); got > 3 {
		t.Errorf("http-header opened %d upstream connection(s) for %d requests, expected pooling\n%s",
			got, n, dumpGoroutines())
	}
	if after := waitGoroutines(t, base, 5, 5*time.Second); after > base+10 {
		t.Logf("goroutine count (diagnostic): base=%d after=%d for %d requests\n%s",
			base, after, n, dumpGoroutines())
	}
}
