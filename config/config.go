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
	"github.com/FMotalleb/go-tools/matcher"
)

type Config struct {
	Core        CoreCfg      `mapstructure:"core" toml:"core" yaml:"core" json:"core"`
	EntryPoints []EntryPoint `mapstructure:"entrypoints" toml:"entrypoints" yaml:"entrypoints" json:"entrypoints"`
}
type CoreCfg struct {
	SingboxCfg map[string]any `mapstructure:"singbox,omitempty" toml:"singbox,omitempty" yaml:"singbox,omitempty" json:"singbox,omitempty"`
}
type EntryPoint struct {
	Routing   Router             `mapstructure:"routing,omitempty" toml:"routing,omitempty" yaml:"routing,omitempty" json:"routing,omitempty"`
	Listen    netip.AddrPort     `mapstructure:"listen,omitempty" toml:"listen,omitempty" yaml:"listen,omitempty" json:"listen,omitempty"`
	BlackList []*matcher.Matcher `mapstructure:"black_list,omitempty" toml:"black_list,omitempty" yaml:"black_list,omitempty" json:"black_list,omitempty"`
	AllowList []*matcher.Matcher `mapstructure:"allow_list,omitempty" toml:"allow_list,omitempty" yaml:"allow_list,omitempty" json:"allow_list,omitempty"`
	Proxy     []*url.URL         `mapstructure:"proxy,omitempty" toml:"proxy,omitempty" yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Target    string             `mapstructure:"to,omitempty" toml:"to,omitempty" yaml:"to,omitempty" json:"to,omitempty"`
	Timeout   time.Duration      `mapstructure:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" json:"timeout,omitempty"`
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

func (e *EntryPoint) Allowed(name string) bool {
	if len(e.BlackList) != 0 {
		for _, b := range e.BlackList {
			if b.Match(name) {
				return false
			}
		}
	}
	if len(e.AllowList) != 0 {
		for _, a := range e.AllowList {
			if a.Match(name) {
				return true
			}
		}
		return false
	}

	return true
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
	default:
		return val, errors.New("there are more than allowed separator (;) in config string")
	}

	return result, nil
}
