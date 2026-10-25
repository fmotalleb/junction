package router

import (
	"context"
	"net"
	"sync"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

func tcpRouter(ctx context.Context, registry *Registry, index int, entry config.EntryPoint) (bool, error) {
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
	active := make(chan struct{}, maxConcurrentTunnels)
	var workers sync.WaitGroup
	defer func() { workers.Wait() }()

	if entry.Target == "" {
		logger.Error("TCP proxy must have a target ip:port address")
		return true, buildFieldMissing("tcp-raw", "to")
	}
	route, _ := registry.Route(index)

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
		select {
		case active <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-active }()
				handleTCPConnection(ctx, logger, conn, entry, route.chain)
			}()
		default:
			logger.Warn("maximum concurrent TCP tunnels reached", zap.Int("limit", maxConcurrentTunnels))
			_ = conn.Close()
		}
	}
}

func handleTCPConnection(parentCtx context.Context, logger *zap.Logger, conn net.Conn, entry config.EntryPoint, chain *proxy.Chain) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()
	defer conn.Close()

	if chain == nil {
		var err error
		chain, err = proxy.NewChainWithDestinationPolicy(entry.Proxy, entry.DestinationAllow, entry.DestinationBlock)
		if err != nil {
			return
		}
	}
	targetConn, err := chain.DialContext(ctx, "tcp", entry.Target)
	if err != nil {
		return
	}
	defer targetConn.Close()

	relayTraffic(ctx, conn, targetConn, logger)
}
