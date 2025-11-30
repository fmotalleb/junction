package config

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"strings"

	"github.com/fmotalleb/go-tools/decoder/hooks"
	"github.com/go-viper/mapstructure/v2"
)

func init() {
	hooks.RegisterHook(StringToNetAddrPortSanitizerHook())
	hooks.RegisterHook(StringToNetAddrSanitizerHook())
	hooks.RegisterHook(IntToNetAddrPortHook())
}

func StringToNetAddrPortSanitizerHook() mapstructure.DecodeHookFunc {
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

func StringToNetAddrSanitizerHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, val interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return val, nil
		}
		if t != reflect.TypeOf(netip.Addr{}) {
			return val, nil
		}
		if str, ok := val.(string); ok {
			if str == "" {
				return netip.Addr{}, nil
			}
			addr, err := netip.ParseAddr(str)
			if err != nil {
				return nil, err
			}
			return addr, nil
		}
		return val, errors.New("expected string value for netip.Addr")
	}
}

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
