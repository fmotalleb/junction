package config

import (
	"fmt"
	"net/url"
)

type Config struct {
	Targets []Target `mapstructure:"targets"`
}

type Target struct {
	ListenPort int     `mapstructure:"port"`
	Proxy      string  `mapstructure:"proxy"`
	Target     url.URL `mapstructure:"target"`
}

func (t *Target) GetListenAddr() string {
	return fmt.Sprintf(":%d", t.ListenPort)
}
