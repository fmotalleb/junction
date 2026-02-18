package router

import (
	"context"
	"net"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

func init() {
	registerHandler(tcpRouter)
}

func tcpRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterTCPRaw {
		return false, nil
	}

	logger := log.FromContext(ctx).
		Named("router.tcp-raw").
		With(zap.Any("entry", entry))

	addrPort := entry.Listen
	tcpAddr := net.TCPAddrFromAddrPort(addrPort)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return true, err
	}
	defer listener.Close()

	if entry.Target == "" {
		logger.Error("TCP proxy must have a target ip:port address")
		return true, buildFieldMissing("tcp-raw", "to")
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
				return true, nil
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

	targetConn, err := dialTarget(entry.Proxy, entry.Target, logger)
	if err != nil {
		return
	}
	defer targetConn.Close()

	relayTraffic(ctx, conn, targetConn, logger)
}
