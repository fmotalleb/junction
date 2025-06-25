package singbox

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/FMotalleb/go-tools/log"
	sb "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"go.uber.org/zap"
)

// Start initializes and runs a Sing-Box instance using the provided configuration.
// It returns an error if the configuration is invalid or if the Sing-Box instance fails to start.
func Start(
	ctx context.Context,
	config map[string]any,
) error {
	log := log.Of(ctx)
	var cfg []byte
	var err error
	var box *sb.Box
	if cfg, err = json.Marshal(config); err != nil {
		return errors.Join(
			errors.New("failed to parse singbox config"),
			err,
		)
	}
	sbCtx := sb.Context(
		ctx,
		include.InboundRegistry(),
		include.OutboundRegistry(),
		include.EndpointRegistry(),
	)
	opt := &sb.Options{
		Context: sbCtx,
	}

	if err = opt.Options.UnmarshalJSONContext(sbCtx, cfg); err != nil {
		return errors.Join(
			errors.New("failed to parse singbox config"),
			err,
		)
	}
	log.Debug("singbox config parsed", zap.Any("config", opt))

	if box, err = sb.New(*opt); err != nil {
		return errors.Join(
			errors.New("failed to create singbox instance"),
			err,
		)
	}
	if err = box.Start(); err != nil {
		return errors.Join(
			errors.New("failed to start singbox instance"),
			err,
		)
	}

	return nil
}
