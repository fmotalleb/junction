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

func sniRouter(ctx context.Context, target config.Target) error {
	if target.Routing != "sni" {
		return nil
	}
	l := log.FromContext(ctx).
		Named("sni-router").
		With(zap.Any("target", target))
	listener, err := net.Listen("tcp", target.GetListenAddr())
	targetPort := "443"
	if target.TargetPort != 0 {
		targetPort = strconv.Itoa(target.TargetPort)
	}
	l.Info("SNI proxy booted")
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			l.Error("failed to accept connection", zap.Error(err))
			continue
		}

		go handleSNIConnection(l, conn, target.Proxy, targetPort)
	}
}

func handleSNIConnection(l *zap.Logger, clientConn net.Conn, proxyAddr, targetPort string) {
	defer clientConn.Close()

	// Read the first few bytes to extract SNI
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
	finalLogger := l.With(zap.String("SNI", serverName))
	finalLogger.Debug("SNI detected")

	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		finalLogger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return
	}

	// Connect to target server through SOCKS5 proxy
	targetConn, err := dialer.Dial("tcp", net.JoinHostPort(serverName, targetPort))
	if err != nil {
		finalLogger.Error("failed to connect to target", zap.Error(err))
		return
	}
	defer targetConn.Close()

	// Send the initial buffer to target
	if _, err := targetConn.Write(buffer[:n]); err != nil {
		finalLogger.Error("failed to write initial buffer to target", zap.Error(err))
		return
	}

	// Start bidirectional copying
	go utils.Copy(clientConn, targetConn)
	utils.Copy(targetConn, clientConn)
}
