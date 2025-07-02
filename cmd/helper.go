package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/FMotalleb/junction/config"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

func marshalData(data any, format string) ([]byte, error) {
	switch format {
	default:
		fallthrough
	case "toml":
		return toml.Marshal(data)
	case "json":
		return json.Marshal(data)
	case "yaml":
		return yaml.Marshal(data)
	}
}

func dumpConf(cfg *config.Config, form ...string) error {
	var err error
	var result []byte
	format := ""
	if len(form) != 0 {
		format = form[0]
	}
	result, err = marshalData(cfg, format)
	if err != nil {
		return fmt.Errorf("failed to encode %s: %w", format, err)
	}
	if result == nil {
		return fmt.Errorf("given format `%s` is not implemented yet, use one of toml,yaml,json", format)
	}
	fmt.Println(string(result))
	return nil
}
