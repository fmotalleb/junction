package cmd

import (
	"encoding/json"
	"errors"

	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

func marshalData(data any, format string) ([]byte, error) {
	switch format {
	case "toml":
		return toml.Marshal(data)
	case "json":
		return json.Marshal(data)
	case "yaml":
		return yaml.Marshal(data)
	}
	return []byte{}, errors.New("unknown format")
}
