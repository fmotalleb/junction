package router

import (
	"context"
	"errors"
	"net"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/proxy"
	"github.com/FMotalleb/junction/utils"
	"go.uber.org/zap"
)

type handler func(context.Context, config.EntryPoint) error

var handlers = []handler{}

func registerHandler(h handler) {
	handlers = append(handlers, h)
}

func Handle(ctx context.Context, e config.EntryPoint) error {
	for _, h := range handlers {
		if err := h(ctx, e); err != nil {
			return errors.Join(
				errors.New("handler denied the configuration"),
				err,
			)
		}
	}
	return errors.New("no handler found for config")
}

func DialTarget(ctx context.Context, proxyAddr, target string, logger *zap.Logger) (net.Conn, error) {
	dialer, err := proxy.NewDialer(proxyAddr)
	if err != nil {
		logger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return nil, err
	}

	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		logger.Error("failed to connect to target", zap.Error(err))
		return nil, err
	}
	return conn, nil
}

func RelayTraffic(src, dst net.Conn, logger *zap.Logger) {
	errCh := make(chan error, 2)

	go func() { errCh <- utils.Copy(dst, src) }()
	go func() { errCh <- utils.Copy(src, dst) }()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			logger.Debug("copy finished", zap.Error(err))
		}
	}
}
