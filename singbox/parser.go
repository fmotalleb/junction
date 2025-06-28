package singbox

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/spf13/cast"
)

// Utility function to get query parameter value with a default.
func getQueryValue(query url.Values, key, defaultValue string) string {
	if val := query.Get(key); val != "" {
		return val
	}
	return defaultValue
}

// TryParseOutboundURL parses the given link and populates the TrojanVLESSBean fields.
func TryParseOutboundURL(url *url.URL) (map[string]any, error) {
	t := new(option.VLESSOutboundOptions)

	query := url.Query()

	t.Server = url.Hostname()
	port := url.Port()
	if port == "" {
		port = "443"
	}
	t.ServerPort = cast.ToUint16(port)

	t.UUID = url.User.Username()

	t.Transport = &option.V2RayTransportOptions{}
	// Security
	t.Transport.Type = getQueryValue(query, "type", "tcp")
	loadTLSParams(query, t)

	// Type
	switch t.Transport.Type {
	case "ws", "http", "httpupgrade":
		t.Transport.HTTPOptions = option.V2RayHTTPOptions{
			Path: getQueryValue(query, "path", ""),
			Host: strings.Split(query.Get("host"), ","),
		}

	case "grpc":
		t.Transport.GRPCOptions = option.V2RayGRPCOptions{
			ServiceName: query.Get("serviceName"),
		}
	}
	t.Flow = query.Get("flow")
	j, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any)
	err = json.Unmarshal(j, &out)
	if err != nil {
		return nil, err
	}
	out["tag"] = "proxy"
	out["type"] = "vless"
	return out, nil
}

func loadTLSParams(query url.Values, t *option.VLESSOutboundOptions) {
	if query.Get("security") != "tls" {
		return
	}
	t.TLS = &option.OutboundTLSOptions{}
	t.TLS.Enabled = true
	insec := query.Get("allowInsecure")
	if insec == "" {
		insec = "0"
	}
	t.TLS.Insecure = cast.ToBool(insec)
	t.TLS.ServerName = query.Get("sni")
	if t.TLS.ServerName == "" {
		t.TLS.ServerName = t.Server
	}
	if query.Get("fp") != "" {
		t.TLS.UTLS = &option.OutboundUTLSOptions{}
		t.TLS.UTLS.Fingerprint = query.Get("fp")
	}
	if query.Get("pbk") != "" {
		t.TLS.Reality = &option.OutboundRealityOptions{}
		t.TLS.Reality.PublicKey = query.Get("pbk")
		t.TLS.Reality.ShortID = query.Get("sid")
		t.TLS.Reality.Enabled = true
	}
}
