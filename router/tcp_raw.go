package router

import (
	"context"
	"net"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

func tcpRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterTCPRaw {
		return false, nil
	}

	logger := log.FromContext(ctx).
		Named("router.tcp-raw").
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
		)

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

	var retry acceptRetry
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("listener closed due to context cancellation")
				return true, nil
			}
			logger.Error("failed to accept connection", zap.Error(err))
			if !retry.wait(ctx) {
				logger.Info("listener closed due to context cancellation")
				return true, nil
			}
			continue
		}
		retry.reset()

		if !entry.AllowedFrom(conn.RemoteAddr()) {
			logger.Debug("connection rejected",
				zap.String("client", conn.RemoteAddr().String()),
			)
			_ = conn.Close()
			continue
		}

		setKeepAlive(conn)
		go handleTCPConnection(ctx, logger, conn, entry)
	}
}

func handleTCPConnection(parentCtx context.Context, logger *zap.Logger, conn net.Conn, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()
	defer conn.Close()

	targetConn, err := dialTarget(ctx, entry.Proxy, entry.Target, logger)
	if err != nil {
		return
	}
	defer targetConn.Close()

	relayTraffic(ctx, conn, targetConn, logger)
}
