package router

import (
	"context"
	"net"
	"strconv"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/utils"
	"github.com/FMotalleb/log"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"
)

func init() {
	registerHandler(sniRouter)
}

func sniRouter(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != "sni" {
		return nil
	}
	l := log.FromContext(ctx).
		Named("router.sni").
		With(zap.Any("entry", entry))

	listenAddr := entry.GetListenAddr()
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		l.Error("failed to listen", zap.String("addr", listenAddr), zap.Error(err))
		return err
	}
	defer listener.Close()

	targetPort := "443"
	if entry.TargetPort != 0 {
		targetPort = strconv.Itoa(entry.TargetPort)
	}
	l.Info("SNI proxy booted", zap.String("listen", listenAddr), zap.String("proxy", entry.Proxy), zap.String("targetPort", targetPort))

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				l.Info("listener closed due to context cancellation")
				return nil
			default:
				l.Error("failed to accept connection", zap.Error(err))
				continue
			}
		}
		go handleSNIConnection(l, conn, entry.Proxy, targetPort)
	}
}

func handleSNIConnection(l *zap.Logger, clientConn net.Conn, proxyAddr, targetPort string) {
	defer clientConn.Close()

	buffer := make([]byte, 4096)
	n, err := clientConn.Read(buffer)
	if err != nil {
		l.Error("failed to read from client", zap.Error(err))
		return
	}

	serverName := utils.ExtractSNI(buffer[:n])
	if serverName == "" {
		l.Error("failed to extract SNI from connection")
		return
	}
	connLogger := l.With(zap.String("SNI", serverName))
	connLogger.Debug("SNI detected")

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		connLogger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return
	}

	targetConn, err := dialer.Dial("tcp", net.JoinHostPort(serverName, targetPort))
	if err != nil {
		connLogger.Error("failed to connect to target", zap.Error(err))
		return
	}
	defer targetConn.Close()

	_, err = targetConn.Write(buffer[:n])
	if err != nil {
		connLogger.Error("failed to write initial buffer to target", zap.Error(err))
		return
	}

	// Bidirectional copy with error handling
	errCh := make(chan error, 2)
	go func() {
		err := utils.Copy(clientConn, targetConn)
		errCh <- err
	}()
	go func() {
		err := utils.Copy(targetConn, clientConn)
		errCh <- err
	}()

	// Wait for one side to finish (close/error)
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			connLogger.Debug("copy finished", zap.Error(err))
		}
	}
}
