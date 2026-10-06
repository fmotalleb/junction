package router

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"testing"
	"time"
)

// startHTTPTarget deliberately keeps idle connections open: an origin that
// never hangs up is exactly the case a leaked transport can not recover from.
func startHTTPTarget(t *testing.T) (addr string, stop func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	l := listenTCP(t)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(l) }()
	return l.Addr().String(), func() { _ = srv.Close() }
}

func TestHTTPHeaderReusesTransport(t *testing.T) {
	target, stopTarget := startHTTPTarget(t)
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

	after := waitGoroutines(t, base, 5, 5*time.Second)
	t.Logf("goroutines base=%d after=%d", base, after)
	if after > base+10 {
		t.Errorf("http-header leaked goroutines: base=%d after=%d for %d requests\n%s",
			base, after, n, dumpGoroutines())
	}
}
