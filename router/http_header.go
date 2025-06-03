package router

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/utils"
	"golang.org/x/net/proxy"
)

func init() {
	registerHandler(httpHandler)
}

func httpHandler(ctx context.Context, target config.Target) error {
	if target.Routing != "http-header" {
		return nil
	}
	targetPort := "80"
	if target.TargetPort != 0 {
		targetPort = strconv.Itoa(target.TargetPort)
	}
	server := &http.Server{
		BaseContext: func(l net.Listener) context.Context {
			return ctx
		},
		Addr: target.GetListenAddr(),
		Handler: &httpProxyHandler{
			proxyAddr:  target.Proxy,
			targetPort: targetPort,
			listenPort: strconv.Itoa(target.ListenPort),
		},
	}

	log.Printf("HTTP proxy listening on port %d", target.ListenPort)
	if err := server.ListenAndServe(); err != nil {
		log.Printf("HTTP server error: %v", err)
		return errors.Join(
			errors.New("failed to start listener for http webserver"),
			err,
		)
	}
	return nil
}

type httpProxyHandler struct {
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
	log.Printf("HTTP request to: %s", targetHost)

	// Handle CONNECT method for HTTPS
	if r.Method == "CONNECT" {
		h.handleConnect(w, r, targetHost)
		return
	}

	// Handle regular HTTP requests
	h.handleHTTP(w, r, targetHost)
}

func (h *httpProxyHandler) handleConnect(w http.ResponseWriter, r *http.Request, targetHost string) {
	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", h.proxyAddr, nil, proxy.Direct)
	if err != nil {
		http.Error(w, "Failed to create SOCKS5 dialer", http.StatusInternalServerError)
		return
	}

	// Connect to target through SOCKS5 proxy
	targetConn, err := dialer.Dial("tcp", targetHost)
	if err != nil {
		http.Error(w, "Failed to connect to target", http.StatusBadGateway)
		return
	}
	defer targetConn.Close()

	// Send 200 Connection Established
	w.WriteHeader(http.StatusOK)

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Start bidirectional copying
	go utils.Copy(clientConn, targetConn)
	utils.Copy(targetConn, clientConn)
}

func (h *httpProxyHandler) handleHTTP(w http.ResponseWriter, r *http.Request, targetHost string) {
	// Parse target URL
	targetURL := &url.URL{
		Scheme:   "http",
		Host:     targetHost,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	// Create SOCKS5 transport
	dialer, err := proxy.SOCKS5("tcp", h.proxyAddr, nil, proxy.Direct)
	if err != nil {
		http.Error(w, "Failed to create SOCKS5 dialer", http.StatusInternalServerError)
		return
	}

	transport := &http.Transport{
		Dial: dialer.Dial,
	}

	client := &http.Client{Transport: transport}

	// Create new request
	req, err := http.NewRequest(r.Method, targetURL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to make request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
