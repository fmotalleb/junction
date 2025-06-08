package config

import (
	"cmp"
	"fmt"
	"time"

	"github.com/FMotalleb/go-tools/env"
)

type Config struct {
	EntryPoints []EntryPoint `mapstructure:"entrypoints"`
}

type EntryPoint struct {
	ListenPort int           `mapstructure:"port"`
	Target     string        `mapstructure:"to"`
	Proxy      string        `mapstructure:"proxy"`
	Routing    string        `mapstructure:"routing"`
	Timeout    time.Duration `mapstructure:"timeout"`
}

func (t *EntryPoint) GetListenAddr() string {
	return fmt.Sprintf(":%d", t.ListenPort)
}

func (t *EntryPoint) IsDirect() bool {
	return t.Proxy == "direct"
}

func (t *EntryPoint) GetTimeout() time.Duration {
	return cmp.Or(
		t.Timeout,
		env.DurationOr("TIMEOUT", time.Hour*24),
	)
}
