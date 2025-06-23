package config

import (
	"cmp"
	"errors"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
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
	Routing Router         `mapstructure:"routing" toml:"routing" yaml:"routing" json:"routing"`
	Timeout time.Duration  `mapstructure:"timeout" toml:"timeout" yaml:"timeout" json:"timeout"`
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

func (e *EntryPoint) Decode(from, _ reflect.Type, val interface{}) (any, error) {
	if from.Kind() != reflect.String {
		return val, nil
	}

	strVal, ok := val.(string)
	if !ok {
		return val, errors.New("expected string value for entrypoint")
	}

	split := strings.Split(strVal, ";")
	result := make(map[string]any, 5)
	result["timeout"] = e.GetTimeout()
	switch len(split) {
	case 5:
		result["timeout"] = split[4]
		fallthrough
	case 4:
		r := make([]string, 0)
		p := strings.Split(split[3], ",")
		for _, proxy := range p {
			if proxy == "" {
				continue
			}
			result["proxy"] = append(r, proxy)
		}
		result["proxy"] = r
		fallthrough
	case 3:
		result["to"] = split[2]
		fallthrough
	case 2:
		result["listen"] = split[1]
		fallthrough
	case 1:
		result["routing"] = split[0]
	}

	return result, nil
}
