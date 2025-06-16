package router

import (
	"context"
	"errors"
	"net"
	"net/url"

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

func DialTarget(proxyAddr []url.URL, target string, logger *zap.Logger) (net.Conn, error) {
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

	go func() {
		err := utils.Copy(dst, src)
		if err != nil {
			errCh <- errors.Join(
				errors.New("failed to write to target connection"),
				err,
			)
		} else {
			errCh <- nil
		}
	}()
	go func() {
		err := utils.Copy(src, dst)
		if err != nil {
			errCh <- errors.Join(
				errors.New("failed to receive from target"),
				err,
			)
		} else {
			errCh <- nil
		}
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			logger.Warn("connection collapsed, one or more connection error", zap.Error(err))
		}
	}
}
