package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FMotalleb/junction/decoder"
	"github.com/spf13/viper"
)

func getConfigType(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "yaml", "yml", "json", "toml", "ini", "hcl":
		return ext
	default:
		return "toml"
	}
}

func Parse(dst *Config, format, path string) error {
	v := viper.New()
	v.SetConfigFile(path)

	cfgType := format
	if format == "" {
		cfgType = getConfigType(path)
	}
	v.SetConfigType(cfgType)

	if path == "" {
		if err := v.ReadConfig(os.Stdin); err != nil {
			return fmt.Errorf("error reading `%s` config from stdin: %w", cfgType, err)
		}
	} else if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	decoder, err := decoder.Build(dst)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(v.AllSettings()); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}
