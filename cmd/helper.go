package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/FMotalleb/junction/config"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// marshalData serializes the given data into the specified format.
// Supported formats are "toml" (default), "json", and "yaml".
// Returns the serialized data as a byte slice or an error if serialization fails.
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

// dumpConf serializes the given configuration into the specified format and prints it to standard output.
// If no format is provided, TOML is used by default. Returns an error if serialization fails or the format is unsupported.
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
