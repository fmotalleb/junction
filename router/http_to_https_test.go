package router

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

func TestRewriteRequestBodyStreamsUnchangedBodies(t *testing.T) {
	r := &http.Request{
		Body:          io.NopCloser(strings.NewReader("binary payload")),
		ContentLength: 14,
		Header:        make(http.Header),
	}
	body, err := rewriteRequestBody(r, nil, 4)
	if err != nil {
		t.Fatal(err)
	}
	if body != r.Body {
		t.Fatal("unchanged request body was buffered or replaced")
	}
}

func TestRewriteRequestBodyBoundsAndRewritesText(t *testing.T) {
	r := &http.Request{
		Body:   io.NopCloser(strings.NewReader("hello")),
		Header: http.Header{"Content-Type": {"text/plain"}},
	}
	body, err := rewriteRequestBody(r, strings.NewReplacer("hello", "goodbye"), 8)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "goodbye" || r.ContentLength != int64(len(got)) {
		t.Fatalf("rewritten body = %q length=%d", got, r.ContentLength)
	}

	large := &http.Request{
		Body:   io.NopCloser(strings.NewReader("0123456789")),
		Header: http.Header{"Content-Type": {"text/plain"}},
	}
	if _, err := rewriteRequestBody(large, strings.NewReplacer("0", "1"), 8); err == nil {
		t.Fatal("oversized request body was accepted for rewriting")
	}
}

func TestDecodeBodyRejectsMalformedAndExpandedGzip(t *testing.T) {
	if _, err := decodeBody("gzip", []byte("not gzip"), 32); err == nil {
		t.Fatal("malformed gzip body was accepted")
	}

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(strings.Repeat("x", 128))); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeBody("gzip", compressed.Bytes(), 32); err == nil {
		t.Fatal("gzip body expanded beyond the configured limit but was accepted")
	}
}

func TestHTTPSProxyStreamsUnsupportedEncodedTextResponse(t *testing.T) {
	const size = maxRewrittenBodyBytes + 1024
	payload := strings.Repeat("x", size)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "br")
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()
	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := proxyTransportForTest(t)
	h := &httpToHTTPSProxy{logger: zap.NewNop()}
	route := httpsRoute{
		entry:        config.EntryPoint{Timeout: 10 * time.Second},
		transport:    transport,
		respReplacer: strings.NewReplacer("origin", "replacement"),
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	h.forward(recorder, route, *targetURL, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if got := recorder.Body.Len(); got != size {
		t.Fatalf("streamed response size = %d, want %d", got, size)
	}
}

func proxyTransportForTest(t *testing.T) *http.Transport {
	t.Helper()
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	return transport
}
