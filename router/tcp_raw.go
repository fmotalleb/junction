package router

import (
	"context"
	"errors"
	"net"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/proxy"
	"github.com/FMotalleb/junction/utils"
	"go.uber.org/zap"
)

func init() {
	registerHandler(tcpRouter)
}

func tcpRouter(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != "tcp-raw" {
		return nil
	}
	l := log.FromContext(ctx).
		Named("router.tcp-raw").
		With(zap.Any("entry", entry))

	listenAddr := entry.GetListenAddr()
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		l.Error("failed to listen", zap.String("addr", listenAddr), zap.Error(err))
		return err
	}
	defer listener.Close()

	targetAddr := entry.Target
	if targetAddr == "" {
		l.Error("TCP proxy must have a target ip:port address")
		return errors.New("router: tcp-raw must have `to` field")
	}

	l.Info("raw TCP proxy booted", zap.String("listen", listenAddr), zap.String("proxy", entry.Proxy), zap.String("target", targetAddr))

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
		go handleTCPConnection(l, conn, entry.Proxy, targetAddr)
	}
}

func handleTCPConnection(l *zap.Logger, clientConn net.Conn, proxyAddr, target string) {
	defer clientConn.Close()

	dialer, err := proxy.NewDialer(proxyAddr)
	if err != nil {
		l.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return
	}

	targetConn, err := dialer.Dial("tcp", target)
	if err != nil {
		l.Error("failed to connect to target", zap.Error(err))
		return
	}
	defer targetConn.Close()

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
			l.Debug("copy finished", zap.Error(err))
		}
	}
}
