package config

import (
	"cmp"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/constants"
	"github.com/fmotalleb/go-tools/env"
	"github.com/fmotalleb/go-tools/matcher"
	"golang.org/x/net/idna"
)

type Config struct {
	Core        CoreCfg      `mapstructure:"core" toml:"core" yaml:"core" json:"core"`
	EntryPoints []EntryPoint `mapstructure:"entrypoints" toml:"entrypoints" yaml:"entrypoints" json:"entrypoints"`
}

// HardenedMode enables the opt-in security controls documented in HARDENING.md.
// Legacy-compatible behavior remains the default unless explicitly enabled.
func HardenedMode() bool {
	return os.Getenv("JUNCTION_HARDENED") == "1"
}

type CoreCfg struct {
	FakeDNS *FakeDNS `mapstructure:"fake_dns" toml:"fake_dns,omitempty" yaml:"fake_dns,omitempty" json:"fake_dns,omitempty"`
}

type EntryPoint struct {
	Routing          Router             `mapstructure:"routing,omitempty" toml:"routing,omitempty" yaml:"routing,omitempty" json:"routing,omitempty"`
	Listen           netip.AddrPort     `mapstructure:"listen,omitempty" toml:"listen,omitempty" yaml:"listen,omitempty" json:"listen,omitempty"`
	BlockList        []*matcher.Matcher `mapstructure:"block_list,omitempty" toml:"block_list,omitempty" yaml:"block_list,omitempty" json:"block_list,omitempty"`
	AllowList        []*matcher.Matcher `mapstructure:"allow_list,omitempty" toml:"allow_list,omitempty" yaml:"allow_list,omitempty" json:"allow_list,omitempty"`
	AllowFrom        []*matcher.Matcher `mapstructure:"allow_from,omitempty" toml:"allow_from,omitempty" yaml:"allow_from,omitempty" json:"allow_from,omitempty"`
	BlockFrom        []*matcher.Matcher `mapstructure:"block_from,omitempty" toml:"block_from,omitempty" yaml:"block_from,omitempty" json:"block_from,omitempty"`
	DestinationAllow []string           `mapstructure:"destination_allow,omitempty" toml:"destination_allow,omitempty" yaml:"destination_allow,omitempty" json:"destination_allow,omitempty"`
	DestinationBlock []string           `mapstructure:"destination_block,omitempty" toml:"destination_block,omitempty" yaml:"destination_block,omitempty" json:"destination_block,omitempty"`
	Proxy            []*url.URL         `mapstructure:"proxy,omitempty" toml:"proxy,omitempty" yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Target           string             `mapstructure:"to,omitempty" toml:"to,omitempty" yaml:"to,omitempty" json:"to,omitempty"`
	ResponseHeaders  map[string]string  `mapstructure:"response_headers,omitempty" toml:"response_headers,omitempty" yaml:"response_headers,omitempty" json:"response_headers,omitempty"`
	Content          string             `mapstructure:"content,omitempty" toml:"content,omitempty" yaml:"content,omitempty" json:"content,omitempty"`
	Timeout          time.Duration      `mapstructure:"timeout,omitempty" toml:"timeout,omitempty" yaml:"timeout,omitempty" json:"timeout,omitempty"`

	// Tag used for grouping entrypoints of auto-router kind
	Tag *string `mapstructure:"tag,omitempty" toml:"tag,omitempty" yaml:"tag,omitempty" json:"tag,omitempty"`

	Features     []string       `mapstructure:"features,omitempty" toml:"features,omitempty" yaml:"features,omitempty" json:"features,omitempty"`
	AllowedPorts []string       `mapstructure:"allowed_ports,omitempty" toml:"allowed_ports,omitempty" yaml:"allowed_ports,omitempty" json:"allowed_ports,omitempty"`
	PublicAccess bool           `mapstructure:"public_access,omitempty" toml:"public_access,omitempty" yaml:"public_access,omitempty" json:"public_access,omitempty"`
	ExtraConf    map[string]any `mapstructure:"extra" toml:"extra,omitempty" yaml:"extra,omitempty" json:"extra,omitempty"`
}

type FakeDNS struct {
	Listen     *netip.AddrPort   `mapstructure:"listen,omitempty" toml:"listen,omitempty" yaml:"listen,omitempty" json:"listen,omitempty"`
	ReturnAddr []*DNSResult      `mapstructure:"answer,omitempty" toml:"answer,omitempty" yaml:"answer,omitempty" json:"answer,omitempty"`
	Forwarder  *netip.AddrPort   `mapstructure:"forwarder,omitempty" toml:"forwarder,omitempty" yaml:"forwarder,omitempty" json:"forwarder,omitempty"`
	Allowed    []matcher.Matcher `mapstructure:"allowed,omitempty" toml:"allowed,omitempty" yaml:"allowed,omitempty" json:"allowed,omitempty"`
	AllowFrom  []matcher.Matcher `mapstructure:"allow_from,omitempty" toml:"allow_from,omitempty" yaml:"allow_from,omitempty" json:"allow_from,omitempty"`
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

// AllowedHost applies host rules to normalized forms of a hostname. Ports are
// removed before matching; IPv4/IPv6 literals, case, trailing dots, and IDNA
// Unicode/ASCII forms therefore do not change the policy result.
func (e *EntryPoint) AllowedHost(authority string) bool {
	candidates := normalizedHostCandidates(authority)
	if len(candidates) == 0 {
		return false
	}
	for _, rule := range e.BlockList {
		for _, candidate := range candidates {
			if rule.Match(candidate) {
				return false
			}
		}
	}
	if len(e.AllowList) == 0 {
		return true
	}
	for _, rule := range e.AllowList {
		for _, candidate := range candidates {
			if rule.Match(candidate) {
				return true
			}
		}
	}
	return false
}

func normalizedHostCandidates(authority string) []string {
	host := strings.TrimSpace(authority)
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil || parsed.Host == "" || parsed.User != nil {
			return nil
		}
		host = parsed.Host
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}
	}
	trimmed := strings.TrimSuffix(host, ".")
	if trimmed == "" {
		return nil
	}
	variants := []string{trimmed, strings.ToLower(trimmed)}
	ascii, err := idna.Lookup.ToASCII(trimmed)
	if err == nil {
		variants = append(variants, strings.ToLower(ascii))
		unicode, unicodeErr := idna.Lookup.ToUnicode(ascii)
		if unicodeErr == nil {
			variants = append(variants, strings.ToLower(strings.TrimSuffix(unicode, ".")))
		}
	}
	unique := make([]string, 0, len(variants))
	seen := make(map[string]struct{}, len(variants))
	for _, variant := range variants {
		if _, exists := seen[variant]; !exists {
			seen[variant] = struct{}{}
			unique = append(unique, variant)
		}
	}
	return unique
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
		r := make([]string, 0, strings.Count(split[3], ",")+1)
		p := strings.Split(split[3], ",")
		for _, proxy := range p {
			if proxy == "" {
				continue
			}
			r = append(r, proxy)
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
