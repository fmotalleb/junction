package router

import (
	"context"
	"net"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/crypto/tls"
	"go.uber.org/zap"
)

const DefaultSNIPort = "443"

func init() {
	registerHandler(sniRouter)
}

func sniRouter(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != config.RouterSNI {
		return nil
	}

	logger := log.FromContext(ctx).
		Named("router.sni").
		With(zap.Any("entry", entry))

	addrPort := entry.Listen
	tcpAddr := net.TCPAddrFromAddrPort(addrPort)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return err
	}
	defer listener.Close()

	logger.Info("SNI proxy booted")

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
		go handleSNIConnection(ctx, logger, conn, entry)
	}
}

func handleSNIConnection(parentCtx context.Context, logger *zap.Logger, clientConn net.Conn, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = clientConn.Close()
	}()

	serverName, buffer, n, err := ReadAndExtractSNI(clientConn, logger)
	if err != nil {
		return
	}

	connLogger := logger.With(zap.ByteString("SNI", serverName))
	connLogger.Debug("SNI detected")

	targetAddr := net.JoinHostPort(string(serverName), entry.GetTargetOr(DefaultSNIPort))
	targetConn, err := DialTarget(entry.Proxy, targetAddr, connLogger)
	if err != nil {
		return
	}
	defer targetConn.Close()

	if _, err := targetConn.Write(buffer[:n]); err != nil {
		connLogger.Error("failed to write initial buffer to target", zap.Error(err))
		return
	}

	RelayTraffic(clientConn, targetConn, connLogger)
}

func ReadAndExtractSNI(conn net.Conn, logger *zap.Logger) ([]byte, []byte, int, error) {
	buffer := make([]byte, 4096)
	n, err := conn.Read(buffer)
	if err != nil {
		logger.Error("failed to read from client", zap.Error(err))
		return nil, nil, 0, err
	}
	// Since we only care about hostname we use this function instead of parsing whole hello packet
	serverName := tls.ExtractSNI(buffer[:n])
	if serverName == nil {
		logger.Error("failed to extract SNI from connection")
		return nil, nil, 0, nil
	}

	return serverName, buffer, n, nil
}
