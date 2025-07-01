package singbox

import (
	"net/url"

	"github.com/FMotalleb/go-tools/builder"
	"github.com/spf13/cast"
)

// TryParseOutboundURL parses the given link and outputs the outbound part of the singbox config.
func TryParseOutboundURL(url *url.URL) (map[string]any, error) {
	cb := builder.NewNested()

	query := url.Query()

	cb.Set("type", url.Scheme).
		Set("tag", "proxy").
		Set("packet_encoding", query.Get("packetEncoding")).
		Set("server", url.Hostname())
	port := url.Port()
	if port == "" {
		port = "443"
	}
	cb.Set("server_port", cast.ToUint16(port))

	if url.User != nil {
		cb.Set("uuid", url.User.Username())
	}
	loadTLSParams(cb, query)

	cb.Set("transport.type", query.Get("type"))
	switch query.Get("type") {
	case "tcp", "ws", "http", "httpupgrade":
		cb.Set("transport.path", query.Get("path"))
		cb.Set("transport.headers.Host", query.Get("host"))
	}
	sn := query.Get("serviceName")
	if sn != "" {
		cb.Set("transport.service_name", query.Get("serviceName"))
	}
	cb.Set("flow", query.Get("flow"))
	out := cb.Data

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

func loadTLSParams(cb *builder.Nested, query url.Values) {
	if query.Get("security") != "tls" {
		return
	}
	cb.Set("tls.enabled", true)

	insec := query.Get("allowInsecure")

	if insec == "" {
		insec = "0"
	}
	cb.Set("tls.insecure", cast.ToBool(insec)).
		Set("tls.server_name", query.Get("sni"))

	if query.Get("fp") != "" {
		cb.Set("tls.utls.enabled", true).
			Set("tls.utls.fingerprint", query.Get("fp"))
	}
	if query.Get("pbk") != "" {
		cb.Set("tls.reality.enabled", true).
			Set("tls.reality.public_key", query.Get("pbk")).
			Set("tls.reality.short_id", query.Get("sid"))
	}
}
