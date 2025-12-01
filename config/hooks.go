package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"reflect"
	"strings"

	"github.com/fmotalleb/go-tools/decoder/hooks"
	"github.com/go-viper/mapstructure/v2"
)

// configuration values into netip.AddrPort and net.IP types.
func init() {
	hooks.RegisterHook(StringToNetAddrPortHook())
	hooks.RegisterHook(StringToNetAddrHook())
	hooks.RegisterHook(IntToNetAddrPortHook())
	hooks.RegisterHook(StringToCIDRHook())
}

// StringToNetAddrPortHook returns a mapstructure.DecodeHookFunc that converts string values into netip.AddrPort.
//
// The hook accepts either "host:port" or "port". If only a port is provided, the host defaults to "127.0.0.1".
// An empty string yields the zero netip.AddrPort. If the input is not a string, the hook returns an error;
// if the string cannot be parsed as an address:port, the hook returns the parsing error.
func StringToNetAddrPortHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, val interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return val, nil
		}
		if t != reflect.TypeOf(netip.AddrPort{}) {
			return val, nil
		}
		if str, ok := val.(string); ok {
			if str == "" {
				return netip.AddrPort{}, nil
			}
			split := strings.Split(str, ":")
			final := make([]string, 2)
			switch len(split) {
			case 1:
				final[0] = ""
				final[1] = split[0]
			case 2:
				final[0] = split[0]
				final[1] = split[1]
			}
			if final[0] == "" {
				final[0] = "127.0.0.1"
			}
			addrPort, err := netip.ParseAddrPort(final[0] + ":" + final[1])
			if err != nil {
				return nil, err
			}
			return addrPort, nil
		}
		return val, errors.New("expected string value for netip.AddrPort")
	}
}

// StringToNetAddrHook returns a mapstructure.DecodeHookFunc that converts string inputs into net.IP values.
//
// The returned hook only acts when the source kind is string and the target type is net.IP. For an empty string it yields nil; for a non-empty string it parses the value with net.ParseIP and returns the resulting net.IP or an error if parsing fails. If the incoming value is not a string, the hook returns an error stating a string was expected. For non-matching source/target types the hook returns the input unchanged.
func StringToNetAddrHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, val interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return val, nil
		}
		if t != reflect.TypeOf(net.IP{}) {
			return val, nil
		}
		if str, ok := val.(string); ok {
			if str == "" {
				return nil, nil
			}
			addr := net.ParseIP(str)
			if addr == nil {
				return nil, fmt.Errorf("failed to parse input '%s' into net.IP", str)
			}
			return addr, nil
		}
		return val, errors.New("expected string value for net.IP")
	}
}

// IntToNetAddrPortHook produces a mapstructure.DecodeHookFunc that converts signed integer values into netip.AddrPort values.
// The hook activates when the source kind is a signed integer and the target type is netip.AddrPort; it formats the integer as the port of "127.0.0.1", parses the resulting "127.0.0.1:<port>" string into a netip.AddrPort, and returns the parsed AddrPort or an error if parsing fails.
func IntToNetAddrPortHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, val interface{}) (interface{}, error) {
		switch f.Kind() {
		case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		default:
			return val, nil
		}
		if t != reflect.TypeOf(netip.AddrPort{}) {
			return val, nil
		}
		strVal := fmt.Sprintf("127.0.0.1:%d", val)
		addrPort, err := netip.ParseAddrPort(strVal)
		if err != nil {
			return nil, err
		}
		return addrPort, nil
	}
}

func StringToCIDRHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, val interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return val, nil
		}
		if str, ok := val.(string); ok {
			if str == "" {
				return nil, nil
			}
			_, addr, err := net.ParseCIDR(str)
			if err != nil {
				return nil, err
			}
			return *addr, nil
		}
		return val, nil
	}
}
