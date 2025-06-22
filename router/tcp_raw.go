package router

import (
	"context"
	"errors"
	"net"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/junction/config"
	"go.uber.org/zap"
)

func init() {
	registerHandler(tcpRouter)
}

func tcpRouter(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != "tcp-raw" {
		return nil
	}

	logger := log.FromContext(ctx).
		Named("router.tcp-raw").
		With(zap.Any("entry", entry))

	addrPort := entry.Listen
	tcpAddr := net.TCPAddrFromAddrPort(addrPort)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return err
	}
	defer listener.Close()

	if entry.Target == "" {
		logger.Error("TCP proxy must have a target ip:port address")
		return errors.New("router: tcp-raw must have `to` field")
	}

	logger.Info("raw TCP proxy booted")

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("listener closed due to context cancellation")
				return nil
			}
			logger.Error("failed to accept connection", zap.Error(err))
			continue
		}

		go handleTCPConnection(ctx, logger, conn, entry)
	}
}

func handleTCPConnection(parentCtx context.Context, logger *zap.Logger, conn net.Conn, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	targetConn, err := DialTarget(entry.Proxy, entry.Target, logger)
	if err != nil {
		return
	}
	defer targetConn.Close()

	RelayTraffic(conn, targetConn, logger)
}
