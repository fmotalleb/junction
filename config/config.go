package config

import (
	"fmt"
)

type Config struct {
	Targets []Target `mapstructure:"targets"`
}

type Target struct {
	ListenPort int    `mapstructure:"port"`
	TargetPort int    `mapstructure:"to"`
	Proxy      string `mapstructure:"proxy"`
	Routing    string `mapstructure:"routing"`
}

func (t *Target) GetListenAddr() string {
	return fmt.Sprintf(":%d", t.ListenPort)
}

func (t *Target) HasProxy() bool {
	return t.Proxy != "direct"
}
