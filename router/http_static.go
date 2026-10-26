package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

type compiledStaticResponse struct {
	content    *template.Template
	headers    map[string]*texttemplate.Template
	statusCode int
}

const httpsScheme = "https"

func compileStaticResponse(entry config.EntryPoint) (*compiledStaticResponse, error) {
	statusCode := entry.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	if statusCode < 200 || statusCode > 599 {
		return nil, errors.New("status_code must be between 200 and 599")
	}
	content, err := template.New("static-content").Option("missingkey=error").Parse(entry.Content)
	if err != nil {
		return nil, fmt.Errorf("parse content template: %w", err)
	}
	headers := make(map[string]*texttemplate.Template, len(entry.ResponseHeaders))
	for name, value := range entry.ResponseHeaders {
		if !validResponseHeaderName(name) {
			return nil, fmt.Errorf("invalid response header name %q", name)
		}
		compiled, err := texttemplate.New(name).Option("missingkey=error").Parse(value)
		if err != nil {
			return nil, fmt.Errorf("parse response header %q template: %w", name, err)
		}
		headers[name] = compiled
	}
	return &compiledStaticResponse{content: content, headers: headers, statusCode: statusCode}, nil
}

func validResponseHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

func httpStaticHandler(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterHTTPStatic {
		return false, nil
	}
	logger := log.FromContext(ctx).Named("router.http-static").With(
		zap.String("router", string(entry.Routing)),
		zap.String("listen", entry.Listen.String()),
	)
	response, err := compileStaticResponse(entry)
	if err != nil {
		return true, err
	}
	server := &http.Server{
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		Addr:              entry.Listen.String(),
		Handler:           &staticHTTPProxy{logger: logger, entry: entry, response: response},
	}
	logger.Info("static HTTP endpoint booted")
	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server error", zap.Error(err))
		return true, fmt.Errorf("failed to start listener for static HTTP endpoint: %w", err)
	}
	return true, nil
}

type staticHTTPProxy struct {
	logger   *zap.Logger
	entry    config.EntryPoint
	response *compiledStaticResponse
}

type staticHTTPTemplateData struct {
	URL    *url.URL
	Method string
}

func (h *staticHTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.entry.AllowedHost(r.Host) || !h.entry.AllowedFrom(addrFromRemote(r.RemoteAddr)) {
		http.Error(w, "request is not allowed", http.StatusForbidden)
		return
	}
	requestURL := *r.URL
	requestURL.Host = r.Host
	if requestURL.Scheme == "" {
		requestURL.Scheme = "http"
		if r.TLS != nil {
			requestURL.Scheme = httpsScheme
		}
	}
	data := staticHTTPTemplateData{URL: &requestURL, Method: r.Method}
	body, err := executeStaticContent(h.response.content, data)
	if err != nil {
		h.logger.Error("failed to render static response content", zap.Error(err))
		http.Error(w, "failed to render response", http.StatusInternalServerError)
		return
	}
	for name, headerTemplate := range h.response.headers {
		value, err := executeStaticHeader(headerTemplate, data)
		if err != nil {
			h.logger.Error("failed to render static response header", zap.String("header", name), zap.Error(err))
			http.Error(w, "failed to render response", http.StatusInternalServerError)
			return
		}
		w.Header().Set(name, value)
	}
	if h.response.statusCode != http.StatusNoContent && h.response.statusCode != http.StatusNotModified {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.WriteHeader(h.response.statusCode)
	if r.Method != http.MethodHead && h.response.statusCode != http.StatusNoContent && h.response.statusCode != http.StatusNotModified {
		_, _ = w.Write(body)
	}
}

func executeStaticContent(t *template.Template, data staticHTTPTemplateData) ([]byte, error) {
	var body bytes.Buffer
	if err := t.Execute(&body, data); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func executeStaticHeader(t *texttemplate.Template, data staticHTTPTemplateData) (string, error) {
	var value strings.Builder
	if err := t.Execute(&value, data); err != nil {
		return "", err
	}
	for _, r := range value.String() {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return "", errors.New("rendered header contains a control character")
		}
	}
	return value.String(), nil
}
