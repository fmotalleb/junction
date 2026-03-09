package config

import (
	"cmp"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/constants"
	"github.com/fmotalleb/go-tools/env"
	"github.com/fmotalleb/go-tools/matcher"
)

type Config struct {
	Core        CoreCfg      `mapstructure:"core" toml:"core" yaml:"core" json:"core"`
	EntryPoints []EntryPoint `mapstructure:"entrypoints" toml:"entrypoints" yaml:"entrypoints" json:"entrypoints"`
}

type CoreCfg struct {
	FakeDNS *FakeDNS `mapstructure:"fake_dns" toml:"fake_dns,omitempty" yaml:"fake_dns,omitempty" json:"fake_dns,omitempty"`
}

type EntryPoint struct {
	Routing   Router             `mapstructure:"routing,omitempty" toml:"routing,omitempty" yaml:"routing,omitempty" json:"routing,omitempty"`
	Listen    netip.AddrPort     `mapstructure:"listen,omitempty" toml:"listen,omitempty" yaml:"listen,omitempty" json:"listen,omitempty"`
	BlockList []*matcher.Matcher `mapstructure:"block_list,omitempty" toml:"block_list,omitempty" yaml:"block_list,omitempty" json:"block_list,omitempty"`
	AllowList []*matcher.Matcher `mapstructure:"allow_list,omitempty" toml:"allow_list,omitempty" yaml:"allow_list,omitempty" json:"allow_list,omitempty"`
	AllowFrom []*matcher.Matcher `mapstructure:"allow_from,omitempty" toml:"allow_from,omitempty" yaml:"allow_from,omitempty" json:"allow_from,omitempty"`
	BlockFrom []*matcher.Matcher `mapstructure:"block_from,omitempty" toml:"block_from,omitempty" yaml:"block_from,omitempty" json:"block_from,omitempty"`
	Proxy     []*url.URL         `mapstructure:"proxy,omitempty" toml:"proxy,omitempty" yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Target    string             `mapstructure:"to,omitempty" toml:"to,omitempty" yaml:"to,omitempty" json:"to,omitempty"`
	Timeout   time.Duration      `mapstructure:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Tag used for grouping entrypoints of auto-router kind
	Tag *string `mapstructure:"tag,omitempty" toml:"tag,omitempty" yaml:"tag,omitempty" json:"tag,omitempty"`

	Features []string `mapstructure:"features,omitempty" toml:"features,omitempty" yaml:"features,omitempty" json:"features,omitempty"`
}

type FakeDNS struct {
	Listen     *netip.AddrPort   `mapstructure:"listen,omitempty" toml:"listen,omitempty" yaml:"listen,omitempty" json:"listen,omitempty"`
	ReturnAddr []*DNSResult      `mapstructure:"answer,omitempty" toml:"answer,omitempty" yaml:"answer,omitempty" json:"answer,omitempty"`
	Forwarder  *netip.AddrPort   `mapstructure:"forwarder,omitempty" toml:"forwarder,omitempty" yaml:"forwarder,omitempty" json:"forwarder,omitempty"`
	Allowed    []matcher.Matcher `mapstructure:"allowed,omitempty" toml:"allowed,omitempty" yaml:"allowed,omitempty" json:"allowed,omitempty"`
}

type DNSResult struct {
	From   []*net.IPNet `mapstructure:"from,omitempty" toml:"from,omitempty" yaml:"from,omitempty" json:"from,omitempty"`
	Result *net.IP      `mapstructure:"answer,omitempty" toml:"answer,omitempty" yaml:"answer,omitempty" json:"answer,omitempty"`
}

func (e *EntryPoint) IsDirect() bool {
	return len(e.Proxy) == 0
}

func (e *EntryPoint) GetTimeout() time.Duration {
	return cmp.Or(
		e.Timeout,
		env.DurationOr("TIMEOUT", constants.Day),
	)
}

func (e *EntryPoint) GetTargetOr(def ...string) string {
	items := append([]string{e.Target}, def...)
	return cmp.Or(items...)
}

func (e *EntryPoint) Allowed(name string) bool {
	if len(e.BlockList) != 0 {
		for _, b := range e.BlockList {
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

func (e *EntryPoint) AllowedFrom(addr net.Addr) bool {
	if addr == nil {
		return len(e.AllowFrom) == 0
	}

	raw := addr.String()
	from := raw
	if host, _, err := net.SplitHostPort(raw); err == nil && host != "" {
		from = host
	}
	if from == "" {
		from = raw
	}

	if len(e.BlockFrom) != 0 {
		for _, b := range e.BlockFrom {
			if b.Match(from) {
				return false
			}
		}
	}
	if len(e.AllowFrom) != 0 {
		for _, a := range e.AllowFrom {
			if a.Match(from) {
				return true
			}
		}
		return false
	}
	return true
}

func (e *EntryPoint) Decode(from reflect.Type, val interface{}) (any, error) {
	if from.Kind() != reflect.String {
		return val, nil
	}

	strVal, ok := val.(string)
	if !ok {
		return val, errors.New("expected string value for entrypoint")
	}

	split := strings.Split(strVal, ";")
	const entryPointsStringSegments = 5
	result := make(map[string]any, entryPointsStringSegments)
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

func (e *FakeDNS) Decode(from reflect.Type, val interface{}) (any, error) {
	if from.Kind() != reflect.String {
		return val, nil
	}

	raw, ok := val.(string)
	if !ok {
		return val, errors.New("expected string value for dns object")
	}

	parts := strings.SplitN(raw, ";", 3)

	result := make(map[string]any, 3)

	switch len(parts) {
	case 3:
		result["listen"] = strings.TrimSpace(parts[0])
		result["answer"] = strings.TrimSpace(parts[1])
		result["forwarder"] = strings.TrimSpace(parts[2])
	case 2:
		result["listen"] = strings.TrimSpace(parts[0])
		result["answer"] = strings.TrimSpace(parts[1])
	case 1:
		result["answer"] = strings.TrimSpace(parts[0])
	}
	return result, nil
}

func (e *DNSResult) Decode(from reflect.Type, val interface{}) (any, error) {
	if from.Kind() != reflect.String {
		return val, nil
	}
	raw, ok := val.(string)
	if !ok {
		return val, nil
	}
	zeroMask := &net.IPNet{
		IP:   net.IPv4(0, 0, 0, 0),
		Mask: net.CIDRMask(0, 32), // /0 mask
	}
	e.From = make([]*net.IPNet, 1)
	e.From[0] = zeroMask

	ans := net.ParseIP(raw)
	if ans == nil {
		return nil, errors.New("failed to parse input string to ip")
	}
	e.Result = &ans
	return e, nil
}
