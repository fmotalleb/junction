package config

import (
	"context"
	"fmt"

	"github.com/fmotalleb/go-tools/config"
	"github.com/fmotalleb/go-tools/decoder"
	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap/zapcore"
)

func Parse(dst *Config, path string, debug bool) error {
	ctx := context.TODO()
	if debug {
		ctx = log.WithNewEnvLoggerForced(
			ctx,
			func(b *log.Builder) *log.Builder {
				return b.
					LevelValue(zapcore.DebugLevel).
					Name("config").
					Development(true)
			},
		)
	}
	cfg, err := config.ReadAndMergeConfig(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to read and merge configs: %w", err)
	}
	decoder, err := decoder.Build(dst)
	if err != nil {
		return fmt.Errorf("create decoder: %w", err)
	}

	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}
