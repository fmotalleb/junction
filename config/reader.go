package config

import (
	"context"
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
		return fmt.Errorf("decode: %w", err)
	}
	*dst = candidate
	return nil
}
