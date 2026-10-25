package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fmotalleb/go-tools/log"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"

	"github.com/fmotalleb/junction/config"
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
func dumpConf(cfg any, form ...string) error {
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

func redactConfig(cfg any) (any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode configuration for redaction: %w", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("decode configuration for redaction: %w", err)
	}
	return redactSecrets(value, ""), nil
}

func redactSecrets(value any, key string) any {
	if isSensitiveConfigKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			redacted[childKey] = redactSecrets(childValue, childKey)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, childValue := range typed {
			redacted[index] = redactSecrets(childValue, key)
		}
		return redacted
	case string:
		if strings.Contains(typed, "://") {
			return config.RedactURL(typed)
		}
	}
	return value
}

func isSensitiveConfigKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), ".", "_"))
	for _, marker := range []string{"password", "passwd", "token", "secret", "private_key", "credential", "api_key", "authorization", "auth"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func buildAppContext() (context.Context, context.CancelFunc, error) {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	ctx, err := log.WithNewEnvLogger(ctx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}
