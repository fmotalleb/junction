package config

import (
	"fmt"
)

type Config struct {
	EntryPoints []EntryPoint `mapstructure:"entrypoints"`
}

type EntryPoint struct {
	ListenPort int    `mapstructure:"port"`
	TargetPort int    `mapstructure:"to"`
	Proxy      string `mapstructure:"proxy"`
	Routing    string `mapstructure:"routing"`
}

func (t *EntryPoint) GetListenAddr() string {
	return fmt.Sprintf(":%d", t.ListenPort)
}

func (t *EntryPoint) IsDirect() bool {
	return t.Proxy == "direct"
}
