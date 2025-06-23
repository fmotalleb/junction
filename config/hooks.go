package config

import (
	"errors"
	"reflect"
	"strings"

	"github.com/FMotalleb/go-tools/decoder"
	"github.com/mitchellh/mapstructure"
)

func init() {
	decoder.RegisterHook(EntryPointHookFunc())
}

func EntryPointHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, val interface{}) (interface{}, error) {
		// Check if the target type is EntryPoint
		if to != reflect.TypeOf(EntryPoint{}) {
			return val, nil
		}

		// Only handle string input
		if from.Kind() != reflect.String {
			return val, nil
		}

		if val == nil {
			return nil, nil
		}

		strVal, ok := val.(string)
		if !ok {
			return val, errors.New("expected string value for entrypoint")
		}

		split := strings.Split(strVal, ";")
		result := make(map[string]any, 0)
		switch len(split) {
		case 5:
			result["timeout"] = split[4]
			fallthrough
		case 4:
			result["proxy"] = split[3]
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
}
