package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/proxy"
	"github.com/FMotalleb/junction/utils"
	"github.com/FMotalleb/log"
	"go.uber.org/zap"
)

func init() {
	registerHandler(httpHandler)
}

func httpHandler(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != "http-header" {
		return nil
	}

	l := log.FromContext(ctx).Named("router.http").With(zap.Any("entry", entry))

	targetPort := "80"
	if entry.TargetPort != 0 {
		targetPort = strconv.Itoa(entry.TargetPort)
	}

	server := &http.Server{
		ReadHeaderTimeout: time.Second * 30,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		Addr: entry.GetListenAddr(),
		Handler: &httpProxyHandler{
			ctx:        ctx,
			logger:     l,
			proxyAddr:  entry.Proxy,
			targetPort: targetPort,
			listenPort: strconv.Itoa(entry.ListenPort),
		},
	}

	l.Info("HTTP proxy listening",
		zap.String("listenAddr", entry.GetListenAddr()),
		zap.String("proxyAddr", entry.Proxy),
		zap.String("targetPort", targetPort),
	)

	if err := server.ListenAndServe(); err != nil {
		l.Error("HTTP server error", zap.Error(err))
		return errors.Join(
			errors.New("failed to start listener for http webserver"),
			err,
		)
	}
	return nil
}

type httpProxyHandler struct {
	ctx        context.Context
	logger     *zap.Logger
	proxyAddr  string
	targetPort string
	listenPort string
}

func (h *httpProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetHost := r.Host
	if targetHost == "" {
		targetHost = r.Header.Get("Host")
	}

	if targetHost == "" {
		h.logger.Warn("No host specified in request")
		http.Error(w, "No host specified", http.StatusBadRequest)
		return
	}

	targetHostSplit := strings.Split(targetHost, ":")
	lt := len(targetHostSplit)
	if h.listenPort == targetHostSplit[lt-1] {
		targetHostSplit[lt-1] = h.targetPort
	} else {
		targetHostSplit = append(targetHostSplit, h.targetPort)
	}
	targetHost = strings.Join(targetHostSplit, ":")

	h.logger.Info("HTTP request received",
		zap.String("method", r.Method),
		zap.String("targetHost", targetHost),
		zap.String("remoteAddr", r.RemoteAddr),
	)

	// Handle CONNECT method for HTTPS
	if r.Method == http.MethodConnect {
		h.handleConnect(w, r, targetHost)
		return
	}
	// Handle regular HTTP requests
	h.handleHTTP(w, r, targetHost)
}

func (h *httpProxyHandler) handleConnect(w http.ResponseWriter, _ *http.Request, targetHost string) {
	dialer, err := proxy.NewDialer(h.proxyAddr)
	if err != nil {
		h.logger.Error("Failed to create SOCKS5 dialer", zap.Error(err))
		http.Error(w, "Failed to create SOCKS5 dialer", http.StatusInternalServerError)
		return
	}

	targetConn, err := dialer.Dial("tcp", targetHost)
	if err != nil {
		h.logger.Error("Failed to connect to target (CONNECT)", zap.String("targetHost", targetHost), zap.Error(err))
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		h.logger.Error("Hijacking not supported")
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		h.logger.Error("Failed to hijack connection", zap.Error(err))
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Bidirectional copy with error handling
	errCh := make(chan error, 2)
	go func() {
		err := utils.Copy(clientConn, targetConn)
		errCh <- err
	}()
	go func() {
		err := utils.Copy(targetConn, clientConn)
		errCh <- err
	}()

	// Wait for one side to finish
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			h.logger.Debug("Copy finished with error", zap.Error(err))
		}
	}
}

func (h *httpProxyHandler) handleHTTP(w http.ResponseWriter, r *http.Request, targetHost string) {
	targetURL := &url.URL{
		Scheme:   "http",
		Host:     targetHost,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	dialer, err := proxy.NewDialer(h.proxyAddr)
	if err != nil {
		h.logger.Error("Failed to create SOCKS5 dialer", zap.Error(err))
		http.Error(w, "Failed to create SOCKS5 dialer", http.StatusInternalServerError)
		return
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(
		h.ctx,
		r.Method,
		targetURL.String(),
		r.Body,
	)
	if err != nil {
		h.logger.Error("Failed to create new HTTP request", zap.Error(err))
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		h.logger.Error("Failed to make HTTP request to target", zap.String("targetURL", targetURL.String()), zap.Error(err))
		http.Error(w, "Failed to make request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("Failed to copy response body", zap.Error(err))
	}
}
