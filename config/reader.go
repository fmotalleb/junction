package config

import (
	"context"
	"fmt"

	"github.com/FMotalleb/go-tools/config"
	"github.com/FMotalleb/go-tools/decoder"
)

func Parse(dst *Config, path string) error {
	ctx := context.TODO()
	cfg, err := config.ReadAndMergeConfig(ctx, path)

	decoder, err := decoder.Build(dst)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}
