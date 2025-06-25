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
	box, err = sb.New(*opt)
	if err != nil {
		return errors.Join(
			errors.New("failed to create singbox instance"),
			err,
		)
	}
	return box.Start()
}
