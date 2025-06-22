package config

import (
	"cmp"
	"net/netip"
	"net/url"
	"time"

	"github.com/FMotalleb/go-tools/env"
)

type Config struct {
	EntryPoints []EntryPoint `mapstructure:"entrypoints" toml:"entrypoints" yaml:"entrypoints" json:"entrypoints"`
}

type EntryPoint struct {
	Listen  netip.AddrPort `mapstructure:"listen" toml:"listen" yaml:"listen" json:"listen"`
	Target  string         `mapstructure:"to" toml:"to" yaml:"to" json:"to"`
	Proxy   []*url.URL     `mapstructure:"proxy" toml:"proxy" yaml:"proxy" json:"proxy"`
	Routing string         `mapstructure:"routing" toml:"routing" yaml:"routing" json:"routing"`
	Timeout time.Duration  `mapstructure:"timeout" toml:"timeout" yaml:"timeout" json:"timeout"`
}

// func (e *EntryPoint) GetListenAddr() netip.AddrPort {
// 	return netip.MustParseAddrPort(e.Listen)
// }

func (e *EntryPoint) IsDirect() bool {
	return len(e.Proxy) == 0
}

func (e *EntryPoint) GetTimeout() time.Duration {
	return cmp.Or(
		e.Timeout,
		env.DurationOr("TIMEOUT", time.Hour*24),
	)
}

func (e *EntryPoint) GetTargetOr(def ...string) string {
	items := append([]string{e.Target}, def...)
	return cmp.Or(items...)
}
