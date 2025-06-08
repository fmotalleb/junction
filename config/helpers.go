package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/FMotalleb/go-tools/env"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

func detectFormatAndSet(v *viper.Viper, path string) error {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "yaml", "yml", "json", "toml", "ini", "hcl":
		v.SetConfigType(ext)
		return nil
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func Parse(dst *Config, path string) error {
	v := viper.New()
	v.SetConfigFile(path)

	if err := detectFormatAndSet(v, path); err != nil {
		return err
	}

	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	decoderConfig := &mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToSliceHookFunc(),
			stringToDurationHookFunc(),
			stringToURLHookFunc(),
		),
		Result:  dst,
		TagName: "mapstructure",
	}

	decoder, err := mapstructure.NewDecoder(decoderConfig)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}

// stringToURLHookFunc converts strings to url.URL.
func stringToURLHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		if from.Kind() != reflect.String || to != reflect.TypeOf(url.URL{}) {
			return data, nil
		}
		parsed, err := url.Parse(data.(string))
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		return *parsed, nil
	}
}

// stringToDurationHookFunc converts strings to time.Duration.
func stringToDurationHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		if from.Kind() != reflect.String || to != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}
		parsed, err := time.ParseDuration(data.(string))
		if err != nil {
			return nil, fmt.Errorf("invalid Duration: %w", err)
		}
		return parsed, nil
	}
}

// stringToSliceHookFunc converts strings to slices.
func stringToSliceHookFunc() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data interface{}) (interface{}, error) {
		if from.Kind() != reflect.String || to.Kind() != reflect.Slice {
			return data, nil
		}
		sep := env.Or("SLICE_SEPARATOR", ",")
		slice := strings.Split(data.(string), sep)
		return slice, nil
	}
}
