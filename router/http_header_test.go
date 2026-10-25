package router

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/fmotalleb/go-tools/matcher"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

const exampleHost = "example.com"

func TestPrepareTargetHost(t *testing.T) {
	tests := []struct {
		name       string
		hostHeader string
		targetPort string
		want       string
		wantErr    bool
	}{
		{
			name:       "simple hostname no port",
			hostHeader: exampleHost,
			targetPort: "",
			want:       exampleHost,
			wantErr:    false,
		},
		{
			name:       "hostname with target port",
			hostHeader: exampleHost,
			targetPort: "8080",
			want:       "example.com:8080",
			wantErr:    false,
		},
		{
			name:       "host header contains scheme",
			hostHeader: "http://example.com",
			targetPort: "",
			want:       exampleHost,
			wantErr:    false,
		},
		{
			name:       "host header contains scheme and port",
			hostHeader: "https://example.com:8443",
			targetPort: "",
			want:       exampleHost,
			wantErr:    false,
		},
		{
			name:       "host header contains host:port",
			hostHeader: "example.com:1234",
			targetPort: "",
			want:       exampleHost,
			wantErr:    false,
		},
		{
			name:       "empty host header",
			hostHeader: "",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid hostname",
			hostHeader: "bad_host_name",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid target port non-numeric",
			hostHeader: exampleHost,
			targetPort: "abc",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid target port out of range",
			hostHeader: exampleHost,
			targetPort: "70000",
			want:       "",
			wantErr:    true,
		},
		{
			name:       "invalid URL in host header",
			hostHeader: "http://[::1",
			targetPort: "",
			want:       "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := prepareTargetHost(tt.hostHeader, tt.targetPort)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTargetPortForRequestUsesCONNECTAuthorityOnlyWithoutFixedTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		host   string
		target string
		want   string
		bad    bool
	}{
		{name: "connect authority port", host: "example.com:9443", want: "9443"},
		{name: "bracketed ipv6 authority port", host: "[2001:db8::1]:9443", want: "9443"},
		{name: "fixed route target takes precedence", host: "example.com:9443", target: "443", want: "443"},
		{name: "missing connect port", host: "example.com", bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{Method: http.MethodConnect, Host: tc.host}
			entry := config.EntryPoint{Target: tc.target}
			got, err := targetPortForRequest(req, entry)
			if tc.bad {
				if err == nil {
					t.Fatalf("targetPortForRequest accepted host %q", tc.host)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("targetPortForRequest() = %q, %v; want %q, nil", got, err, tc.want)
			}
		})
	}
}

func TestHandleConnectChecksHijackSupportBeforeDial(t *testing.T) {
	chain, err := proxy.NewChain(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &httpProxyHandler{ctx: context.Background(), logger: zap.NewNop()}
	request := httptest.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	response := httptest.NewRecorder()

	// An invalid network address would fail dialing with 502. An unsupported
	// ResponseWriter must be rejected first, before attempting that dial.
	handler.handleConnect(response, request, "\x00", config.EntryPoint{}, chain)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("CONNECT status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestCopyHeadersWithReplaceDropsUntrustedForwardingAndHopHeaders(t *testing.T) {
	src := http.Header{
		"Forwarded":           {"for=198.51.100.8"},
		"X-Forwarded-For":     {"198.51.100.8"},
		"X-Forwarded-Host":    {"attacker.example"},
		"X-Real-Ip":           {"198.51.100.8"},
		"Proxy-Authorization": {"Basic c2VjcmV0"},
		"Connection":          {"X-Private-Hop"},
		"X-Private-Hop":       {"secret"},
		"X-Request-Id":        {"request-1"},
	}
	dst := make(http.Header)
	copyHeadersWithReplace(dst, src, nil)
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Real-Ip", "Proxy-Authorization", "X-Private-Hop"} {
		if values := dst.Values(name); len(values) != 0 {
			t.Errorf("forwarded %s = %v, want absent", name, values)
		}
	}
	if got := dst.Get("X-Request-Id"); got != "request-1" {
		t.Errorf("X-Request-Id = %q, want request-1", got)
	}
}

func TestHTTPProxyDoesNotFollowRedirectOutsideAllowList(t *testing.T) {
	var deniedReached atomic.Bool
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		deniedReached.Store(true)
		_, _ = io.WriteString(w, "denied origin reached")
	}))
	defer denied.Close()
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, denied.URL, http.StatusFound)
	}))
	defer allowed.Close()

	host, port, err := net.SplitHostPort(allowed.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	allow := new(matcher.Matcher)
	compiled, err := allow.Decode(reflect.TypeOf(""), "glob:"+host)
	if err != nil {
		t.Fatal(err)
	}
	allow = compiled.(*matcher.Matcher)
	chain, err := proxy.NewChain(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := newProxyTransport(chain)
	defer transport.CloseIdleConnections()
	handler := &httpProxyHandler{
		ctx:       context.Background(),
		logger:    zap.NewNop(),
		chain:     chain,
		transport: transport,
		entry:     config.EntryPoint{Target: port, AllowList: []*matcher.Matcher{allow}},
	}
	request := httptest.NewRequest(http.MethodGet, allowed.URL, nil)
	request.RemoteAddr = "127.0.0.1:55555"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("redirect response status = %d, want %d", response.Code, http.StatusFound)
	}
	if deniedReached.Load() {
		t.Fatal("request followed redirect to a disallowed origin")
	}
}

func BenchmarkPrepareTargetHost(b *testing.B) {
	type benchCase struct {
		hostHeader string
		targetPort string
	}

	cases := []benchCase{
		{exampleHost, ""},
		{exampleHost, "8080"},
		{"http://example.com", ""},
		{"https://example.com:443", ""},
		{"example.com:1234", ""},
		{"bad_host_name", ""},
	}

	for _, bc := range cases {
		b.Run(bc.hostHeader+"_"+bc.targetPort, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = prepareTargetHost(bc.hostHeader, bc.targetPort)
			}
		})
	}
}
