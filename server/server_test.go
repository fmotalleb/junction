package server

import (
	"context"
	"net/netip"
	"reflect"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/matcher"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/fmotalleb/junction/config"
)

func mustMatcher(t *testing.T, pattern string) *matcher.Matcher {
	t.Helper()
	m := new(matcher.Matcher)
	out, err := m.Decode(reflect.TypeOf(""), pattern)
	if err != nil {
		t.Fatalf("compile matcher %q: %v", pattern, err)
	}
	return out.(*matcher.Matcher)
}

func TestWarnOpenListener(t *testing.T) {
	for _, tc := range []struct {
		name      string
		listen    string
		allowFrom []string
		wantWarns int
	}{
		{"public listener without allow_from warns", "0.0.0.0:8443", nil, 1},
		{"public listener with allow_from stays quiet", "0.0.0.0:8443", []string{"127.0.0.1"}, 0},
		{"loopback listener stays quiet", "127.0.0.1:8443", nil, 0},
		{"unspecified ipv6 warns", "[::]:8443", nil, 1},
		{"unset listen stays quiet", "", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zap.WarnLevel)
			ctx := log.WithLogger(context.Background(), zap.New(core))

			var listen netip.AddrPort
			if tc.listen != "" {
				listen = netip.MustParseAddrPort(tc.listen)
			}
			entry := config.EntryPoint{Listen: listen}
			for _, pattern := range tc.allowFrom {
				entry.AllowFrom = append(entry.AllowFrom, mustMatcher(t, pattern))
			}

			warnOpenListener(ctx, entry)
			assert.Equal(t, tc.wantWarns, logs.Len())
			if tc.wantWarns > 0 {
				assert.Contains(t, logs.All()[0].Message, "allow_from")
			}
		})
	}
}

func TestServeEmptyConfig(t *testing.T) {
	assert.IsError(t, Serve(context.Background(), config.Config{}), ErrServerDiedBeforeContext)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.NoError(t, Serve(ctx, config.Config{}))
}

func TestBuildBackoff(t *testing.T) {
	assert.True(t, BuildBackoff() != nil)
}
