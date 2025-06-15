package config

import (
	"cmp"
	"fmt"
	"net/url"
	"time"

	"github.com/FMotalleb/go-tools/env"
)

type Config struct {
	EntryPoints []EntryPoint `mapstructure:"entrypoints"`
}

type EntryPoint struct {
	ListenPort int           `mapstructure:"port"`
	Target     string        `mapstructure:"to"`
	Proxy      []url.URL     `mapstructure:"proxy"`
	Routing    string        `mapstructure:"routing"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

func (e *EntryPoint) GetListenAddr() string {
	return fmt.Sprintf("0.0.0.0:%d", e.ListenPort)
}

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
