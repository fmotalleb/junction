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
	"sync"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

var (
	httpsGroupMu sync.Mutex
	httpsGroups  = map[string][]config.EntryPoint{}
)

func init() {
	registerHandler(httpToHTTPSHandler)
	registerReset(func() {
		httpsGroupMu.Lock()
		httpsGroups = make(map[string][]config.EntryPoint)
		httpsGroupMu.Unlock()
	})
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
//	Entry.Tag           - optional tag to group multiple entries under one listener
func httpToHTTPSHandler(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterHTTPToHTTPS {
		return false, nil
	}

	// --- Tag registration ---
	if entry.Tag != nil {
		isFirst := registerHTTPSTaggedEntry(*entry.Tag, entry)
		if !isFirst {
			// Not the first entry → do not start another listener.
			return true, nil
		}
	}

	logger := log.FromContext(ctx).
		Named("router.http_to_https").
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
			zap.String("target", entry.Target),
		)

	if entry.Target == "" && entry.Tag == nil {
		return true, errors.New("http_to_https: entry.Target is required (or use Tag to group entries), e.g. https://google.com")
	}

	targetURL, reqReplacer, respReplacer, err := buildHTTPSEntryConfig(entry)
	if err != nil {
		return true, err
	}
	if targetURL != nil && targetURL.Scheme != "https" {
		logger.Warn("target scheme is not https", zap.String("scheme", targetURL.Scheme))
	}

	server := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler: &httpToHTTPSProxy{
			ctx:          ctx,
			logger:       logger,
			entry:        entry,
			tag:          entry.Tag,
			targetURL:    targetURL,
			reqReplacer:  reqReplacer,
			respReplacer: respReplacer,
		},
	}

	logger.Info("HTTP->HTTPS proxy booted", zap.Any("target", targetURL))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("HTTP server error", zap.Error(err))
		return true, errors.Join(
			errors.New("failed to start listener for http_to_https proxy"),
			err,
		)
	}
	return true, nil
}

func registerHTTPSTaggedEntry(tag string, entry config.EntryPoint) bool {
	httpsGroupMu.Lock()
	defer httpsGroupMu.Unlock()

	group, ok := httpsGroups[tag]
	if !ok {
		httpsGroups[tag] = []config.EntryPoint{entry}
		return true // first entry → should start server
	}

	httpsGroups[tag] = append(group, entry)
	return false // listener already running
}

// buildHTTPSEntryConfig parses the target URL and host replacers from an EntryPoint.
// Returns nil targetURL without error when entry.Target is empty (valid in tag-group
// non-first entries that are only stored, not used to boot the server).
func buildHTTPSEntryConfig(entry config.EntryPoint) (*url.URL, *strings.Replacer, *strings.Replacer, error) {
	var targetURL *url.URL
	if entry.Target != "" {
		var err error
		targetURL, err = url.Parse(entry.Target)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("http_to_https: invalid target URL %q: %w", entry.Target, err)
		}
	}

	var reqReplacements, respReplacements []string
	if replaceHosts, ok := entry.ExtraConf["replace_host"]; ok {
		replaceMap, ok := replaceHosts.(map[string]any)
		if !ok {
			return nil, nil, nil, errors.New("invalid replace map structure, expected string -> string map")
		}
		for upstream, localAddr := range replaceMap {
			local, ok := localAddr.(string)
			if !ok {
				return nil, nil, nil, errors.New("invalid replace map structure, expected string -> string map, right side does not look like a string")
			}
			if upstream == "" || local == "" {
				continue
			}
			reqReplacements = append(reqReplacements, local, upstream)
			respReplacements = append(respReplacements, upstream, local)
		}
	}

	return targetURL, strings.NewReplacer(reqReplacements...), strings.NewReplacer(respReplacements...), nil
}

type httpToHTTPSProxy struct {
	ctx          context.Context
	logger       *zap.Logger
	entry        config.EntryPoint
	tag          *string
	targetURL    *url.URL
	reqReplacer  *strings.Replacer
	respReplacer *strings.Replacer
}

func (h *httpToHTTPSProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remoteAddr := addrFromRemote(r.RemoteAddr)

	// Resolve the effective entry, targetURL, and replacers.
	entry := h.entry
	targetURL := h.targetURL
	reqReplacer := h.reqReplacer
	respReplacer := h.respReplacer

	if h.tag == nil {
		// No tag: single-entry path, check AllowedFrom directly.
		if !entry.AllowedFrom(remoteAddr) {
			h.logger.Debug("connection rejected", zap.String("client", r.RemoteAddr))
			w.WriteHeader(http.StatusForbidden)
			return
		}
	} else {
		// Tag group: find the first entry that matches both host and client.
		// The target URL and replacers come from the matched entry, not the
		// handler's boot-time entry (which may be a different backend).
		httpsGroupMu.Lock()
		group := httpsGroups[*h.tag]
		httpsGroupMu.Unlock()

		matched := false
		// targetHost is the Host header the client sent; for HTTPS reverse-proxy
		// we match on the incoming Host (what the client thinks it's talking to).
		incomingHost := strings.TrimSpace(r.Host)
		if incomingHost == "" {
			incomingHost = r.Header.Get("Host")
		}

		for _, ep := range group {
			if ep.Allowed(incomingHost) && ep.AllowedFrom(remoteAddr) {
				// Rebuild per-entry config so each tagged entry can point at a
				// distinct HTTPS backend with its own replacers.
				var err error
				targetURL, reqReplacer, respReplacer, err = buildHTTPSEntryConfig(ep)
				if err != nil {
					h.logger.Error("failed to build config for matched tag entry", zap.Error(err))
					http.Error(w, "internal configuration error", http.StatusInternalServerError)
					return
				}
				entry = ep
				matched = true
				break
			}
		}

		if !matched {
			h.logger.Warn("no matching entry for https request",
				zap.String("host", incomingHost),
				zap.String("client", r.RemoteAddr),
			)
			w.WriteHeader(http.StatusForbidden)
			return
		}
	}

	if targetURL == nil {
		h.logger.Error("no target URL resolved for request")
		http.Error(w, "no target configured", http.StatusInternalServerError)
		return
	}

	// Build upstream URL
	upstreamURL := *targetURL
	upstreamURL.Path = singleJoiningSlash(targetURL.Path, r.URL.Path)
	upstreamURL.RawQuery = r.URL.RawQuery

	// Rewrite request body if it has replaceable content
	var reqBody io.ReadCloser = r.Body
	if r.Body != nil && reqReplacer != nil {
		if isTextContentType(r.Header.Get("Content-Type")) {
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) > 0 {
				replaced := reqReplacer.Replace(string(bodyBytes))
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
	copyHeadersWithReplace(req.Header, r.Header, reqReplacer)
	req.Host = targetURL.Host
	req.Header.Set("Host", targetURL.Host)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Proto", "http")

	// Transport with optional SOCKS5 dialer
	dialer, err := proxy.NewDialer(entry.Proxy)
	if err != nil {
		http.Error(w, "SOCKS5 dialer error", http.StatusInternalServerError)
		return
	}
	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   entry.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
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
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Content-Encoding") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, respReplacer.Replace(v))
		}
	}

	// Handle Location redirects
	if loc := resp.Header.Get("Location"); loc != "" {
		w.Header().Set("Location", respReplacer.Replace(loc))
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Error("Response read failed", zap.Error(err))
		http.Error(w, "Upstream read failed", http.StatusBadGateway)
		return
	}

	// Decompress if gzipped, then rewrite
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		if gr, err := gzip.NewReader(bytes.NewReader(body)); err == nil {
			uncompressed, _ := io.ReadAll(gr)
			gr.Close()
			body = uncompressed
		}
	}

	if isTextContentType(resp.Header.Get("Content-Type")) && respReplacer != nil {
		body = []byte(respReplacer.Replace(string(body)))
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
