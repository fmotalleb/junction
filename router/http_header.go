package router

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
	"github.com/fmotalleb/junction/utils"
)

const (
	DefaultHTTPPort   = ""
	maxHostnameLength = 255

	flexiblePortFeature = "flexible-port"
)

var (
	httpGroupMu          sync.Mutex
	httpGroups           = map[string][]config.EntryPoint{} // tag → entry list
	validHostnameRfc1123 = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)
	localhostIdentifiers = []string{
		"localhost",
		"localhost.localdomain",
		"localhost6.localdomain6",
		"ip6-localhost",
	}
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
func httpHandler(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterHTTPHeader {
		return false, nil
	}

	// --- Tag registration ---
	if entry.Tag != nil {
		isFirst := registerHTTPTaggedEntry(*entry.Tag, entry)
		if !isFirst {
			// Not the first entry → do not start another listener.
			return true, nil
		}
	}

	logger := log.FromContext(ctx).Named("router.http").With(zap.Any("entry", entry))
	features := slices.Clone(entry.Features)

	server := &http.Server{
		ReadHeaderTimeout: time.Second * 30,
		BaseContext:       func(_ net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler: &httpProxyHandler{
			ctx:          ctx,
			logger:       logger,
			proxyAddr:    entry.Proxy,
			targetPort:   entry.GetTargetOr(DefaultHTTPPort),
			entry:        entry,
			tag:          entry.Tag, // NEW FIELD
			flexiblePort: utils.PopInPlace(&features, flexiblePortFeature),
		},
	}
	//nolint:gocritic // utils.PopInPlace removes the items from array so if the list of features is normal it will be empty here
	if len(features) >= 0 {
		logger.Warn("unused features in entrypoint", zap.Strings("features", features))
	}
	logger.Info("HTTP proxy booted")

	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server error", zap.Error(err))
		return true, errors.Join(
			errors.New("failed to start listener for http proxy"),
			err,
		)
	}

	return true, nil
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
	ctx          context.Context
	logger       *zap.Logger
	proxyAddr    []*url.URL
	targetPort   string
	entry        config.EntryPoint
	tag          *string // NEW
	flexiblePort bool
}

func (h *httpProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	port := h.targetPort
	if h.flexiblePort {
		port = cmp.Or(r.Header.Get("Junction-Port"), port)
	}
	targetHost, err := prepareTargetHost(
		cmp.Or(r.Host, r.Header.Get("Host")),
		port,
	)
	if err != nil {
		h.logger.Warn("failed to prepare target host", zap.Error(err))
		http.Error(w, "malformed host value, refusing to process request", http.StatusBadRequest)
		return
	} else if targetHost == "" {
		h.logger.Warn("failed to read target host")
		http.Error(w, "malformed host value, failed to read the value, refusing to process request", http.StatusBadRequest)
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

	h.logger.Debug("HTTP request received",
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

func prepareTargetHost(hostHeader, targetPort string) (string, error) {
	host := strings.TrimSpace(hostHeader)
	if host == "" {
		return "", errors.New("host header is empty")
	}

	// Only parse URL if scheme exists
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid URL in host header: %w", err)
		}
		host = u.Host
	}

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if err := isValidHostname(host); err != nil {
		return "", err
	}

	if targetPort == "" {
		return host, nil
	}

	buf := make([]byte, 0, len(host)+1+len(targetPort))
	buf = append(buf, host...)
	buf = append(buf, ':')
	buf = append(buf, targetPort...)
	return string(buf), nil
}

// ValidHostname determines whether the passed string is a valid hostname.
// In case it's not, the returned error contains the details of the failure.
// From: https://github.com/datadog/datadog-agent/blob/914b7646d5d4/pkg/util/hostname/validate/validate.go#L16C1-L55C2
func isValidHostname(hostname string) error {
	switch {
	case hostname == "":
		return errors.New("hostname is empty")
	case isLocal(hostname):
		return fmt.Errorf("%s is a local hostname", hostname)
	case len(hostname) > maxHostnameLength:
		return fmt.Errorf("name exceeded the maximum length of %d characters", maxHostnameLength)
	case !validHostnameRfc1123.MatchString(hostname):
		return fmt.Errorf("%s is not RFC1123 compliant", hostname)
	default:
		return nil
	}
}

// check whether the name is in the list of local hostnames.
func isLocal(name string) bool {
	name = strings.ToLower(name)
	for _, val := range localhostIdentifiers {
		if val == name {
			return true
		}
	}
	return false
}

func (h *httpProxyHandler) handleConnect(w http.ResponseWriter, _ *http.Request, targetHost string) {
	ctx, cancel := context.WithTimeout(h.ctx, h.entry.Timeout)
	defer cancel()
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

	relayTraffic(ctx, clientConn, targetConn, h.logger)
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
