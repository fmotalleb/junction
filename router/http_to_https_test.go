package router

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
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
