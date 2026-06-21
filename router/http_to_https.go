package router

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

func init() {
	registerHandler(httpToHTTPSHandler)
}

// httpToHTTPSHandler starts an HTTP server that reverse-proxies to an HTTPS backend.
// Routing: config.RouterHTTPToHTTPS
//
// Expected config.EntryPoint fields:
//
//	Entry.Listen        - where to listen, e.g. ":80"
//	Entry.Target        - https backend URL, e.g. "https://google.com"
//	Entry.ReplaceHost   - map[upstream_host]local_host
//	                      Example: {"google.com": "127.0.0.1"}
//	                      Request: 127.0.0.1 -> google.com
//	                      Response: google.com -> 127.0.0.1
//	Entry.Proxy         - optional SOCKS5 chain, same as http_header.go
//	Entry.Timeout       - upstream timeout
func httpToHTTPSHandler(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterHTTPToHTTPS {
		return false, nil
	}

	logger := log.FromContext(ctx).
		Named("router.http_to_https").
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
			zap.String("target", entry.Target),
		)

	if entry.Target == "" {
		return true, errors.New("http_to_https: entry.Target is required, e.g. https://google.com")
	}

	targetURL, err := url.Parse(entry.Target)
	if err != nil {
		return true, fmt.Errorf("http_to_https: invalid target URL: %w", err)
	}
	if targetURL.Scheme != "https" {
		logger.Warn("target scheme is not https", zap.String("scheme", targetURL.Scheme))
	}

	// Build replacers
	// ReplaceHost is defined as map[upstream]local
	// reqReplacer:  local -> upstream
	// respReplacer: upstream -> local
	var reqReplacements, respReplacements []string
	if replaceHosts, ok := entry.ExtraConf["replace_host"]; ok {
		var replaceMap map[string]any
		if replaceMap, ok = replaceHosts.(map[string]any); !ok {
			return true, errors.New("invalid replace map structure, expected string -> string map")
		}
		for upstream, localAddr := range replaceMap {
			var local string
			if local, ok = localAddr.(string); !ok {
				return true, errors.New("invalid replace map structure, expected string -> string map, right side does not look like a string")
			}
			if upstream == "" || local == "" {
				continue
			}
			// request: client sends local, we send upstream
			reqReplacements = append(reqReplacements, local, upstream)
			// response: backend sends upstream, we send local
			respReplacements = append(respReplacements, upstream, local)
		}
	}
	reqReplacer := strings.NewReplacer(reqReplacements...)
	respReplacer := strings.NewReplacer(respReplacements...)

	server := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler: &httpToHTTPSProxy{
			ctx:          ctx,
			logger:       logger,
			entry:        entry,
			targetURL:    targetURL,
			reqReplacer:  reqReplacer,
			respReplacer: respReplacer,
		},
	}

	logger.Info("HTTP->HTTPS proxy booted", zap.String("target", targetURL.String()))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server error", zap.Error(err))
		return true, errors.Join(
			errors.New("failed to start listener for http_to_https proxy"),
			err,
		)
	}
	return true, nil
}

type httpToHTTPSProxy struct {
	ctx          context.Context
	logger       *zap.Logger
	entry        config.EntryPoint
	targetURL    *url.URL
	reqReplacer  *strings.Replacer
	respReplacer *strings.Replacer
}

func (h *httpToHTTPSProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remoteAddr := addrFromRemote(r.RemoteAddr)

	if !h.entry.AllowedFrom(remoteAddr) {
		h.logger.Debug("connection rejected", zap.String("client", r.RemoteAddr))
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Build upstream URL
	upstreamURL := *h.targetURL
	upstreamURL.Path = singleJoiningSlash(h.targetURL.Path, r.URL.Path)
	upstreamURL.RawQuery = r.URL.RawQuery

	// Rewrite request body if it has replaceable content
	var reqBody io.ReadCloser = r.Body
	if r.Body != nil && h.reqReplacer != nil {
		if isTextContentType(r.Header.Get("Content-Type")) {
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) > 0 {
				replaced := h.reqReplacer.Replace(string(bodyBytes))
				reqBody = io.NopCloser(strings.NewReader(replaced))
				r.ContentLength = int64(len(replaced))
			}
		}
	}

	req, err := http.NewRequestWithContext(h.ctx, r.Method, upstreamURL.String(), reqBody)
	if err != nil {
		h.logger.Error("Request creation failed", zap.Error(err))
		http.Error(w, "Request creation failed", http.StatusInternalServerError)
		return
	}

	// Copy headers, with Host rewriting
	copyHeadersWithReplace(req.Header, r.Header, h.reqReplacer)
	req.Host = h.targetURL.Host
	req.Header.Set("Host", h.targetURL.Host)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Proto", "http")

	// Transport with optional SOCKS5 dialer
	dialer, err := proxy.NewDialer(h.entry.Proxy)
	if err != nil {
		http.Error(w, "SOCKS5 dialer error", http.StatusInternalServerError)
		return
	}
	transport := &http.Transport{
		Dial: dialer.Dial,
	}
	// If you want to allow self-signed backends, add a config flag for this:
	// transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: h.entry.Insecure}

	client := &http.Client{
		Transport: transport,
		Timeout:   h.entry.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow, let client handle
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Request to target failed", zap.String("url", upstreamURL.String()), zap.Error(err))
		http.Error(w, "Request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers with replacement
	for k, vv := range resp.Header {
		// Skip hop-by-hop / encoding headers we'll re-compute
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Content-Encoding") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, h.respReplacer.Replace(v))
		}
	}

	// Handle Location redirects
	if loc := resp.Header.Get("Location"); loc != "" {
		w.Header().Set("Location", h.respReplacer.Replace(loc))
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("Response read failed", zap.Error(err))
		http.Error(w, "Upstream read failed", http.StatusBadGateway)
		return
	}

	// Decompress if gzipped, then rewrite
	contentEncoding := resp.Header.Get("Content-Encoding")
	if strings.Contains(contentEncoding, "gzip") {
		if gr, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			uncompressed, _ := io.ReadAll(gr)
			gr.Close()
			body = uncompressed
			// We will serve uncompressed, so don't set Content-Encoding
		}
	}

	if isTextContentType(resp.Header.Get("Content-Type")) && h.respReplacer != nil {
		body = []byte(h.respReplacer.Replace(string(body)))
	}

	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func copyHeadersWithReplace(dst, src http.Header, replacer *strings.Replacer) {
	for k, vv := range src {
		// skip hop-by-hop headers
		if isHopHeader(k) {
			continue
		}
		for _, v := range vv {
			if replacer != nil {
				v = replacer.Replace(v)
			}
			dst.Add(k, v)
		}
	}
}

func isHopHeader(k string) bool {
	switch strings.ToLower(k) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailers", "transfer-encoding", "upgrade", "accept-encoding":
		return true
	default:
		return false
	}
}

func isTextContentType(ct string) bool {
	ct = strings.ToLower(ct)
	if ct == "" {
		return true // be permissive for unknown
	}
	textTypes := []string{
		"text/", "application/json", "application/javascript",
		"application/xml", "application/x-www-form-urlencoded",
		"application/graphql",
	}
	for _, t := range textTypes {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return false
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
