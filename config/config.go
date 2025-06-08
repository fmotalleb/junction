package config

import (
	"fmt"
	"time"
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
