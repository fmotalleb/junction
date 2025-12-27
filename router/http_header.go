package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
	"go.uber.org/zap"
)

const DefaultHTTPPort = ""

var (
	httpGroupMu sync.Mutex
	httpGroups  = map[string][]config.EntryPoint{} // tag → entry list
)

func init() {
	registerHandler(httpHandler)
	registerReset(func() {
		httpGroupMu.Lock()
		httpGroups = make(map[string][]config.EntryPoint)
		httpGroupMu.Unlock()
	})
}

// httpHandler starts an HTTP proxy server for entry points configured with RouterHTTPHeader routing.
// It initializes the server with a proxy handler that forwards requests through a SOCKS5 proxy chain as specified by the entry configuration.
// Returns an error if the server fails to start.
func httpHandler(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != config.RouterHTTPHeader {
		return nil
	}

	// --- Tag registration ---
	if entry.Tag != nil {
		isFirst := registerHTTPTaggedEntry(*entry.Tag, entry)
		if !isFirst {
			// Not the first entry → do not start another listener.
			return nil
		}
	}

	logger := log.FromContext(ctx).Named("router.http").With(zap.Any("entry", entry))

	server := &http.Server{
		ReadHeaderTimeout: time.Second * 30,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler: &httpProxyHandler{
			ctx:        ctx,
			logger:     logger,
			proxyAddr:  entry.Proxy,
			targetPort: entry.GetTargetOr(DefaultHTTPPort),
			entry:      entry,
			tag:        entry.Tag, // NEW FIELD
		},
	}

	logger.Info("HTTP proxy booted")

	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server error", zap.Error(err))
		return errors.Join(
			errors.New("failed to start listener for http proxy"),
			err,
		)
	}

	return nil
}

func registerHTTPTaggedEntry(tag string, entry config.EntryPoint) bool {
	httpGroupMu.Lock()
	defer httpGroupMu.Unlock()

	group, ok := httpGroups[tag]
	if !ok {
		httpGroups[tag] = []config.EntryPoint{entry}
		return true // first entry → should start server
	}

	httpGroups[tag] = append(group, entry)
	return false // listener already running
}

type httpProxyHandler struct {
	ctx        context.Context
	logger     *zap.Logger
	proxyAddr  []*url.URL
	targetPort string
	entry      config.EntryPoint
	tag        *string // NEW
}

func (h *httpProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetHost := prepareTargetHost(r.Host, r.Header.Get("Host"), h.targetPort)
	if targetHost == "" {
		h.logger.Warn("No host specified in request")
		http.Error(w, "No host specified", http.StatusBadRequest)
		return
	}

	// --- Tag group selection logic ---
	entry := h.entry
	if h.tag != nil {
		group := httpGroups[*h.tag]

		for _, ep := range group {
			if ep.Allowed(targetHost) {
				entry = ep
				break
			}
		}
	}

	if !entry.Allowed(targetHost) {
		h.logger.Warn("hostname rejected", zap.String("hostname", targetHost))
		w.WriteHeader(http.StatusForbidden)
		return
	}

	h.logger.Info("HTTP request received",
		zap.String("method", r.Method),
		zap.String("targetHost", targetHost),
		zap.String("remoteAddr", r.RemoteAddr),
	)

	if r.Method == http.MethodConnect {
		h.handleConnect(w, r, targetHost)
	} else {
		h.handleHTTPRequest(w, r, targetHost)
	}
}

func prepareTargetHost(hostHeader, fallback string, targetPort string) string {
	host := hostHeader
	if host == "" {
		host = fallback
	}
	if host == "" {
		return ""
	}
	if targetPort == "" {
		return host
	}
	hostname, _, _ := net.SplitHostPort(host)
	return net.JoinHostPort(hostname, targetPort)
}

func (h *httpProxyHandler) handleConnect(w http.ResponseWriter, _ *http.Request, targetHost string) {
	dialer, err := proxy.NewDialer(h.proxyAddr)
	if err != nil {
		http.Error(w, "SOCKS5 dialer error", http.StatusInternalServerError)
		return
	}

	targetConn, err := dialer.Dial("tcp", targetHost)
	if err != nil {
		h.logger.Error("CONNECT failed", zap.String("target", targetHost), zap.Error(err))
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		h.logger.Error("Hijacking unsupported")
		http.Error(w, "Hijacking unsupported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		h.logger.Error("Hijack failed", zap.Error(err))
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	relayTraffic(clientConn, targetConn, h.logger)
}

func (h *httpProxyHandler) handleHTTPRequest(w http.ResponseWriter, r *http.Request, targetHost string) {
	dialer, err := proxy.NewDialer(h.proxyAddr)
	if err != nil {
		http.Error(w, "SOCKS5 dialer error", http.StatusInternalServerError)
		return
	}

	targetURL := &url.URL{
		Scheme:   "http",
		Host:     targetHost,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	req, err := http.NewRequestWithContext(h.ctx, r.Method, targetURL.String(), r.Body)
	if err != nil {
		h.logger.Error("Request creation failed", zap.Error(err))
		http.Error(w, "Request creation failed", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, v := range r.Header {
		for _, val := range v {
			req.Header.Add(k, val)
		}
	}

	resp, err := (&http.Client{Transport: &http.Transport{Dial: dialer.Dial}}).Do(req)
	if err != nil {
		h.logger.Error("Request to target failed", zap.String("url", targetURL.String()), zap.Error(err))
		http.Error(w, "Request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("Response copy failed", zap.Error(err))
	}
}
