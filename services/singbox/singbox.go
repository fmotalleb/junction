package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/fmotalleb/go-tools/env"
	"github.com/fmotalleb/go-tools/log"
	sb "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sethvargo/go-retry"
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
	cfg = normalizeConfig(cfg)
	sbCtx := sb.Context(
		ctx,
		include.InboundRegistry(),
		include.OutboundRegistry(),
		include.EndpointRegistry(),
		include.DNSTransportRegistry(),
		include.ServiceRegistry(),
	)
	opt := &sb.Options{
		Context: sbCtx,
	}

	if err = opt.UnmarshalJSONContext(sbCtx, cfg); err != nil {
		return errors.Join(
			errors.New("failed to parse singbox config"),
			err,
		)
	}

	for _, i := range opt.Outbounds {
		vless, ok := (i.Options.(*option.VLESSOutboundOptions))
		if !ok {
			continue
		}
		if vless.TLS != nil && !vless.TLS.Enabled {
			vless.TLS = nil
		}
	}
	log.Debug("singbox config parsed", zap.Any("config", opt))

	if box, err = sb.New(*opt); err != nil {
		return errors.Join(
			errors.New("failed to create singbox instance"),
			err,
		)
	}

	if err = box.Start(); err != nil {
		return retry.RetryableError(err)
	}

	// Wait for context cancellation to shutdown gracefully
	<-ctx.Done()
	log.Info("shutting down singbox")
	return box.Close()
}

func normalizeConfig(in []byte) []byte {
	r := regexp.
		MustCompile(`"(\d+|true|false)"`)
	mapped := env.Subst(string(in))
	return []byte(r.
		ReplaceAllStringFunc(
			mapped,
			func(match string) string {
				return strings.ReplaceAll(match, "\"", "")
			},
		),
	)
}
