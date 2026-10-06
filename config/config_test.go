package config_test

import (
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/fmotalleb/go-tools/constants"
	"github.com/fmotalleb/go-tools/matcher"

	"github.com/fmotalleb/junction/config"
)

const (
	exampleSubdomain = "*.example.com"
	apiSubdomain     = "api.example.com"
	exampleDomain    = "example.com"
	googleRegex      = "regex:google"
	blockedDomain    = "bad.example.com"
	tenZeroNetwork   = "10.0.*"
	tenZeroClient    = "10.0.0.7"
)

// mustMatcher compiles a configuration pattern the same way the decoder does.
func mustMatcher(t *testing.T, pattern string) *matcher.Matcher {
	t.Helper()
	m := new(matcher.Matcher)
	out, err := m.Decode(reflect.TypeOf(""), pattern)
	if err != nil {
		t.Fatalf("compile matcher %q: %v", pattern, err)
	}
	return out.(*matcher.Matcher)
}

// fakeAddr lets tests feed a raw "host:port" to AllowedFrom.
type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

func entryWith(t *testing.T, block, allow, blockFrom, allowFrom []string) config.EntryPoint {
	t.Helper()
	e := config.EntryPoint{}
	for _, p := range block {
		e.BlockList = append(e.BlockList, mustMatcher(t, p))
	}
	for _, p := range allow {
		e.AllowList = append(e.AllowList, mustMatcher(t, p))
	}
	for _, p := range blockFrom {
		e.BlockFrom = append(e.BlockFrom, mustMatcher(t, p))
	}
	for _, p := range allowFrom {
		e.AllowFrom = append(e.AllowFrom, mustMatcher(t, p))
	}
	return e
}

func TestEntryPointAllowed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block []string
		allow []string
		host  string
		want  bool
	}{
		{"no lists allows every host", nil, nil, "anything.example.org", true},
		{"allow list accepts a subdomain", nil, []string{exampleSubdomain}, apiSubdomain, true},
		{"allow list rejects another host", nil, []string{exampleSubdomain}, "evil.com", false},
		{"wildcard does not match the base domain", nil, []string{exampleSubdomain}, exampleDomain, false},
		{"exact pattern accepts", nil, []string{exampleDomain}, exampleDomain, true},
		{"regex pattern is a contain check", nil, []string{googleRegex}, "www.google.com", true},
		{"regex pattern rejects", nil, []string{googleRegex}, "www.bing.com", false},
		{"block list rejects", []string{blockedDomain}, nil, blockedDomain, false},
		{"block list leaves other hosts", []string{blockedDomain}, nil, "good.example.com", true},
		{"block wins over allow", []string{apiSubdomain}, []string{exampleSubdomain}, apiSubdomain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := entryWith(t, tc.block, tc.allow, nil, nil)
			assert.Equal(t, tc.want, e.Allowed(tc.host))
		})
	}
}

func TestEntryPointAllowedFrom(t *testing.T) {
	for _, tc := range []struct {
		name      string
		blockFrom []string
		allowFrom []string
		addr      net.Addr
		want      bool
	}{
		{"nil client with no allow_from is allowed", nil, nil, nil, true},
		{"nil client is rejected once allow_from exists", nil, []string{"127.0.0.1"}, nil, false},
		{"loopback is allowed by exact pattern", nil, []string{"127.0.0.1"}, fakeAddr("127.0.0.1:51000"), true},
		{"wildcard covers the subnet", nil, []string{tenZeroNetwork}, fakeAddr("10.0.0.7:1234"), true},
		{"other subnet is rejected", nil, []string{tenZeroNetwork}, fakeAddr("192.168.1.5:1234"), false},
		{"regex pattern matches", nil, []string{`regex:^192\.168\.1\.`}, fakeAddr("192.168.1.9:53"), true},
		{"block_from wins over allow_from", []string{tenZeroClient}, []string{tenZeroNetwork}, fakeAddr("10.0.0.7:1234"), false},
		{"block_from alone rejects the listed client", []string{tenZeroClient}, nil, fakeAddr("10.0.0.7:1234"), false},
		{"block_from alone allows everyone else", []string{tenZeroClient}, nil, fakeAddr("10.0.0.8:1234"), true},
		{"matcher sees the host, not the port", nil, []string{tenZeroClient}, fakeAddr("10.0.0.7:65535"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := entryWith(t, nil, nil, tc.blockFrom, tc.allowFrom)
			assert.Equal(t, tc.want, e.AllowedFrom(tc.addr))
		})
	}
}

// TestMatcherPrefixValidation documents which prefixes the decoder accepts;
// README used to advertise "regexp:" which the matcher package rejects.
func TestMatcherPrefixValidation(t *testing.T) {
	for _, pattern := range []string{"plain.example.com", exampleSubdomain, "glob:*.example.com", googleRegex, "grep:google"} {
		m := new(matcher.Matcher)
		_, err := m.Decode(reflect.TypeOf(""), pattern)
		assert.NoError(t, err, "pattern %q should compile", pattern)
	}
	m := new(matcher.Matcher)
	_, err := m.Decode(reflect.TypeOf(""), "regexp:google")
	assert.Error(t, err, "regexp is not a supported prefix")
}

func TestEntryPointGetTimeout(t *testing.T) {
	e := config.EntryPoint{Timeout: 90 * time.Second}
	assert.Equal(t, 90*time.Second, e.GetTimeout())

	t.Setenv("TIMEOUT", "7s")
	e = config.EntryPoint{}
	assert.Equal(t, 7*time.Second, e.GetTimeout())

	t.Setenv("TIMEOUT", "")
	e = config.EntryPoint{}
	assert.Equal(t, constants.Day, e.GetTimeout())
}

func TestEntryPointGetTargetOr(t *testing.T) {
	e := config.EntryPoint{Target: "443"}
	assert.Equal(t, "443", e.GetTargetOr("8443"))

	e = config.EntryPoint{}
	assert.Equal(t, "8443", e.GetTargetOr("8443"))
	assert.Equal(t, "8443", e.GetTargetOr("", "8443"))
}

func TestEntryPointIsDirect(t *testing.T) {
	plain := config.EntryPoint{}
	assert.True(t, plain.IsDirect())
	proxied := config.EntryPoint{Proxy: []*url.URL{{Scheme: "socks5", Host: "127.0.0.1:7890"}}}
	assert.False(t, proxied.IsDirect())
}

func TestEntryPointDecode(t *testing.T) {
	from := reflect.TypeOf("")

	t.Run("routing only keeps the default timeout", func(t *testing.T) {
		e := new(config.EntryPoint)
		got, err := e.Decode(from, "sni")
		assert.NoError(t, err)
		res, ok := got.(map[string]any)
		assert.True(t, ok)
		assert.Equal(t, "sni", res["routing"])
		assert.Equal[any](t, e.GetTimeout(), res["timeout"])
	})

	t.Run("five segments fill every field", func(t *testing.T) {
		e := new(config.EntryPoint)
		got, err := e.Decode(from, "sni;127.0.0.1:8443;example.com;socks5://127.0.0.1:7890,http://10.0.0.1:8080;45s")
		assert.NoError(t, err)
		res := got.(map[string]any)
		assert.Equal(t, "sni", res["routing"])
		assert.Equal(t, "127.0.0.1:8443", res["listen"])
		assert.Equal(t, exampleDomain, res["to"])
		assert.Equal(t, "45s", res["timeout"])
		assert.Equal[any](t, []string{"socks5://127.0.0.1:7890", "http://10.0.0.1:8080"}, res["proxy"])
	})

	t.Run("empty proxy entries are dropped", func(t *testing.T) {
		e := new(config.EntryPoint)
		got, err := e.Decode(from, "sni;:8080;;socks5://127.0.0.1:7890,")
		assert.NoError(t, err)
		res := got.(map[string]any)
		assert.Equal[any](t, []string{"socks5://127.0.0.1:7890"}, res["proxy"])
	})

	t.Run("too many segments are rejected", func(t *testing.T) {
		e := new(config.EntryPoint)
		_, err := e.Decode(from, "sni;:8080;example.com;proxy;30s;extra")
		assert.Error(t, err)
	})

	t.Run("non string input passes through", func(t *testing.T) {
		e := new(config.EntryPoint)
		got, err := e.Decode(reflect.TypeOf(0), 42)
		assert.NoError(t, err)
		assert.Equal(t, 42, got)
	})

	t.Run("non string value is rejected", func(t *testing.T) {
		e := new(config.EntryPoint)
		_, err := e.Decode(from, 42)
		assert.Error(t, err)
	})
}

func TestDNSResultDecode(t *testing.T) {
	e := new(config.DNSResult)
	got, err := e.Decode(reflect.TypeOf(""), "10.10.10.10")
	assert.NoError(t, err)
	assert.Equal[any](t, e, got)
	assert.Equal(t, "10.10.10.10", e.Result.String())
	assert.Equal(t, 1, len(e.From))
	ones, bits := e.From[0].Mask.Size()
	assert.Equal(t, 0, ones)
	assert.Equal(t, 32, bits)

	_, err = e.Decode(reflect.TypeOf(""), "not-an-ip")
	assert.Error(t, err)
}
