package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/fmotalleb/go-tools/decoder"
)

func Parse(ctx context.Context, dst *Config, path string) error {
	cfg, err := loadConfig(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to read and merge configs: %w", err)
	}
	var candidate Config
	decoder, err := decoder.Build(&candidate)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(cfg); err != nil {
		// Decoder errors may include the rejected input value. Keep the error
		// actionable without echoing configuration contents that may be secrets.
		return errors.New("decode configuration: invalid field value (details omitted to avoid exposing secrets)")
	}
	*dst = candidate
	return nil
}
