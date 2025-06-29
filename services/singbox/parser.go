package singbox

import (
	"net/url"
	"strings"

	"dario.cat/mergo"
	"github.com/spf13/cast"
)

func field(key string, value any) any {
	o := make(map[string]any)
	p := &o
	chain := strings.Split(key, ".")
	cs := len(chain)
	for i, key := range chain {
		if i == cs-1 {
			(*p)[key] = value
		} else {
			c := make(map[string]any)
			(*p)[key] = c
			p = &c
		}
	}
	return o
}

type configBuilder struct {
	frames []any
}

func (c *configBuilder) set(key string, value any) {
	c.frames = append(c.frames, field(key, value))
}

func (c *configBuilder) build() (map[string]any, error) {
	out := make(map[string]any, 0)
	for _, f := range c.frames {
		if err := mergo.Merge(&out, f); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// TryParseOutboundURL parses the given link and outputs the outbound part of the singbox config.
func TryParseOutboundURL(url *url.URL) (map[string]any, error) {
	cb := new(configBuilder)

	query := url.Query()

	cb.set("type", url.Scheme)
	cb.set("tag", "proxy")
	cb.set("packet_encoding", query.Get("packetEncoding"))
	cb.set("server", url.Hostname())
	port := url.Port()
	if port == "" {
		port = "443"
	}
	cb.set("server_port", cast.ToUint16(port))

	if url.User != nil {
		cb.set("uuid", url.User.Username())
	}
	loadTLSParams(cb, query)

	cb.set("transport.type", query.Get("type"))
	switch query.Get("type") {
	case "tcp", "ws", "http", "httpupgrade":
		cb.set("transport.path", query.Get("path"))
		cb.set("transport.headers.Host", query.Get("host"))
	}
	sn := query.Get("serviceName")
	if sn != "" {
		cb.set("transport.service_name", query.Get("serviceName"))
	}
	cb.set("flow", query.Get("flow"))
	out, err := cb.build()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"core": map[string]any{
			"singbox": map[string]any{
				"outbounds": []map[string]any{
					out,
				},
			},
		},
	}, nil
}

func loadTLSParams(cb *configBuilder, query url.Values) {
	if query.Get("security") != "tls" {
		return
	}
	cb.set("tls.enabled", true)

	insec := query.Get("allowInsecure")

	if insec == "" {
		insec = "0"
	}
	cb.set("tls.insecure", cast.ToBool(insec))
	cb.set("tls.server_name", query.Get("sni"))

	if query.Get("fp") != "" {
		cb.set("tls.utls.enabled", true)
		cb.set("tls.utls.fingerprint", query.Get("fp"))
	}
	if query.Get("pbk") != "" {
		cb.set("tls.reality.enabled", true)
		cb.set("tls.reality.public_key", query.Get("pbk"))
		cb.set("tls.reality.short_id", query.Get("sid"))
	}
}
