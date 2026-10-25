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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

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
func httpToHTTPSHandler(ctx context.Context, registry *Registry, index int, entry config.EntryPoint) (bool, error) {
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

	if entry.Target == "" && entry.Tag == nil {
		return true, errors.New("http_to_https: entry.Target is required (or use Tag to group entries), e.g. https://google.com")
	}
	compiled, prepared := registry.Route(index)
	if !prepared {
		compiled.entry = entry
		var err error
		compiled.chain, err = proxy.NewChainWithDestinationPolicy(entry.Proxy, entry.DestinationAllow, entry.DestinationBlock)
		if err != nil {
			return true, err
		}
		compiled.targetURL, compiled.reqReplacer, compiled.respReplacer, err = buildHTTPSEntryConfig(entry)
		if err != nil {
			return true, err
		}
		compiled.transport = newProxyTransport(compiled.chain)
	}
	if compiled.targetURL != nil && (compiled.targetURL.Scheme != "https" || compiled.targetURL.Host == "" || compiled.targetURL.User != nil) {
		return true, errors.New("http_to_https target must be an HTTPS URL with a host and no userinfo")
	}

	server := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler: &httpToHTTPSProxy{
			ctx:       ctx,
			logger:    logger,
			entry:     entry,
			registry:  registry,
			tag:       entry.Tag,
			baseRoute: httpsRoute{entry: entry, targetURL: compiled.targetURL, reqReplacer: compiled.reqReplacer, respReplacer: compiled.respReplacer, chain: compiled.chain, transport: compiled.transport},
		},
	}

	logger.Info("HTTP->HTTPS proxy booted", zap.Any("target", compiled.targetURL))
	if err := serveHTTPServer(ctx, server); err != nil {
		logger.Error("HTTP server error", zap.Error(err))
		return true, errors.Join(
			errors.New("failed to start listener for http_to_https proxy"),
			err,
		)
	}
	return true, nil
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
		upstreams := make([]string, 0, len(replaceMap))
		for upstream := range replaceMap {
			upstreams = append(upstreams, upstream)
		}
		sort.Strings(upstreams)
		for _, upstream := range upstreams {
			localAddr := replaceMap[upstream]
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

	if len(reqReplacements) == 0 {
		return targetURL, nil, nil, nil
	}
	return targetURL, strings.NewReplacer(reqReplacements...), strings.NewReplacer(respReplacements...), nil
}

type httpToHTTPSProxy struct {
	ctx       context.Context
	logger    *zap.Logger
	entry     config.EntryPoint
	registry  *Registry
	tag       *string
	baseRoute httpsRoute
}

type httpsRoute struct {
	entry        config.EntryPoint
	targetURL    *url.URL
	reqReplacer  *strings.Replacer
	respReplacer *strings.Replacer
	chain        *proxy.Chain
	transport    *http.Transport
}

func (h *httpToHTTPSProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := h.resolveRoute(w, r)
	if !ok {
		return
	}
	if route.targetURL == nil {
		h.logger.Error("no target URL resolved for request")
		http.Error(w, "no target configured", http.StatusInternalServerError)
		return
	}

	upstreamURL := *route.targetURL
	upstreamURL.Path = singleJoiningSlash(route.targetURL.Path, r.URL.Path)
	upstreamURL.RawQuery = r.URL.RawQuery

	body, err := rewriteRequestBody(r, route.reqReplacer, maxRewrittenBodyBytes)
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), body)
	if err != nil {
		h.logger.Error("Request creation failed", zap.Error(err))
		http.Error(w, "Request creation failed", http.StatusInternalServerError)
		return
	}
	copyHeadersWithReplace(req.Header, r.Header, route.reqReplacer)
	req.Host = route.targetURL.Host
	req.Header.Set("Host", route.targetURL.Host)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Proto", "http")

	h.forward(w, route, upstreamURL, req)
}

// resolveRoute picks the entrypoint that serves this request: the single
// untagged entry, or the first entry of the tagged group whose Host and client
// address both match. It writes the rejection response itself and reports
// ok=false when the request must not be forwarded.
func (h *httpToHTTPSProxy) resolveRoute(w http.ResponseWriter, r *http.Request) (httpsRoute, bool) {
	remoteAddr := addrFromRemote(r.RemoteAddr)
	route := h.baseRoute

	if h.tag == nil {
		if !route.entry.AllowedFrom(remoteAddr) {
			h.logger.Debug("connection rejected", zap.String("client", r.RemoteAddr))
			w.WriteHeader(http.StatusForbidden)
			return route, false
		}
		return route, true
	}

	group := h.registry.GroupRoutes(*h.tag)

	// targetHost is the Host header the client sent; for HTTPS reverse-proxy
	// we match on the incoming Host (what the client thinks it's talking to).
	incomingHost := strings.TrimSpace(r.Host)
	if incomingHost == "" {
		incomingHost = r.Header.Get("Host")
	}

	for _, compiled := range group {
		ep := compiled.entry
		if !ep.Allowed(incomingHost) || !ep.AllowedFrom(remoteAddr) {
			continue
		}
		route = httpsRoute{
			entry:        ep,
			targetURL:    compiled.targetURL,
			reqReplacer:  compiled.reqReplacer,
			respReplacer: compiled.respReplacer,
			chain:        compiled.chain,
			transport:    compiled.transport,
		}
		return route, true
	}

	h.logger.Warn("no matching entry for https request",
		zap.String("host", incomingHost),
		zap.String("client", r.RemoteAddr),
	)
	w.WriteHeader(http.StatusForbidden)
	return route, false
}

// rewriteRequestBody returns the body to forward, replacing text payloads in
// flight and keeping ContentLength in sync with the rewritten size.
const maxRewrittenBodyBytes = 8 << 20

func rewriteRequestBody(r *http.Request, reqReplacer *strings.Replacer, limit int64) (io.ReadCloser, error) {
	if r.Body == nil || reqReplacer == nil || r.Header.Get("Content-Encoding") != "" || !isTextContentType(r.Header.Get("Content-Type")) {
		return r.Body, nil
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bodyBytes)) > limit {
		return nil, fmt.Errorf("request body exceeds rewrite limit of %d bytes", limit)
	}
	if len(bodyBytes) == 0 {
		return r.Body, nil
	}
	replaced := reqReplacer.Replace(string(bodyBytes))
	r.ContentLength = int64(len(replaced))
	return io.NopCloser(strings.NewReader(replaced)), nil
}

// forward sends the upstream request and mirrors the response back to the
// client, applying the response replacers on the way.
func (h *httpToHTTPSProxy) forward(w http.ResponseWriter, route httpsRoute, upstreamURL url.URL, req *http.Request) {
	transport := route.transport
	if transport == nil {
		chain, err := proxy.NewChainWithDestinationPolicy(route.entry.Proxy, route.entry.DestinationAllow, route.entry.DestinationBlock)
		if err != nil {
			http.Error(w, "proxy route unavailable", http.StatusInternalServerError)
			return
		}
		transport = newProxyTransport(chain)
		defer transport.CloseIdleConnections()
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   route.entry.GetTimeout(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	//nolint:gosec // The upstream host comes from the entrypoint config, only
	// the path and query are client supplied.
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Request to target failed", zap.String("url", upstreamURL.String()), zap.Error(err))
		http.Error(w, "Request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if route.respReplacer == nil || !isTextContentType(resp.Header.Get("Content-Type")) {
		copyResponseHeaders(w, resp.Header, nil, false)
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			h.logger.Error("Response copy failed", zap.Error(err))
		}
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRewrittenBodyBytes+1))
	if err != nil {
		h.logger.Error("Response read failed", zap.Error(err))
		http.Error(w, "Upstream read failed", http.StatusBadGateway)
		return
	}
	if int64(len(body)) > maxRewrittenBodyBytes {
		http.Error(w, "upstream response too large to rewrite", http.StatusBadGateway)
		return
	}
	contentEncoding := resp.Header.Get("Content-Encoding")
	if strings.EqualFold(contentEncoding, "gzip") {
		body, err = decodeBody(contentEncoding, body, maxRewrittenBodyBytes)
		if err != nil {
			http.Error(w, "upstream response could not be decoded", http.StatusBadGateway)
			return
		}
	} else if contentEncoding != "" {
		copyResponseHeaders(w, resp.Header, nil, false)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
		return
	}
	body = replaceTextBody(resp.Header.Get("Content-Type"), body, route.respReplacer)
	copyResponseHeaders(w, resp.Header, route.respReplacer, true)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

func decodeBody(contentEncoding string, body []byte, limit int64) ([]byte, error) {
	if !strings.EqualFold(contentEncoding, "gzip") {
		return body, nil
	}
	gr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	uncompressed, err := io.ReadAll(io.LimitReader(gr, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(uncompressed)) > limit {
		return nil, fmt.Errorf("decompressed response exceeds %d bytes", limit)
	}
	return uncompressed, nil
}

func replaceTextBody(contentType string, body []byte, replacer *strings.Replacer) []byte {
	if replacer == nil || !isTextContentType(contentType) {
		return body
	}
	return []byte(replacer.Replace(string(body)))
}

func copyResponseHeaders(w http.ResponseWriter, headers http.Header, replacer *strings.Replacer, transformed bool) {
	connectionHeaders := connectionHeaderNames(headers)
	for k, vv := range headers {
		if isHopHeader(k) || connectionHeaders[strings.ToLower(k)] || (transformed && strings.EqualFold(k, "Content-Length")) || (transformed && strings.EqualFold(k, "Content-Encoding")) {
			continue
		}
		for _, v := range vv {
			if replacer != nil {
				v = replacer.Replace(v)
			}
			w.Header().Add(k, v)
		}
	}
	if loc := headers.Get("Location"); loc != "" {
		if replacer != nil {
			loc = replacer.Replace(loc)
		}
		w.Header().Set("Location", loc)
	}
}

func copyHeadersWithReplace(dst, src http.Header, replacer *strings.Replacer) {
	connectionHeaders := connectionHeaderNames(src)
	for k, vv := range src {
		// skip hop-by-hop headers
		if isHopHeader(k) || connectionHeaders[strings.ToLower(k)] {
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
		"te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func connectionHeaderNames(headers http.Header) map[string]bool {
	result := make(map[string]bool)
	for _, value := range headers.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
				result[name] = true
			}
		}
	}
	return result
}

func isTextContentType(ct string) bool {
	ct = strings.ToLower(ct)
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
