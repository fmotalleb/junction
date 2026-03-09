package router

import (
	"context"
	"errors"
	"net"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/connection"
)

func init() {
	registerHandler(udpRouter)
}

func udpRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterUDPRaw {
		return false, nil
	}

	logger := log.FromContext(ctx).
		Named("router.udp-raw").
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
		)

	addrPort := entry.Listen
	udpAddr := net.UDPAddrFromAddrPort(addrPort)
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return true, err
	}
	defer conn.Close()

	if entry.Target == "" {
		logger.Error("UDP proxy must have a target ip:port address")
		return true, buildFieldMissing("udp-raw", "to")
	}
	host, _, err := net.SplitHostPort(entry.Target)
	if err != nil || host == "" {
		logger.Error("UDP proxy target must be ip:port", zap.String("target", entry.Target), zap.Error(err))
		return true, buildFieldMissing("udp-raw", "to")
	}
	if !allowedTarget(entry, host, entry.Target) {
		logger.Warn("target blocked by allow/block list", zap.String("target", entry.Target))
		return true, errors.New("target blocked by allow/block list")
	}

	logger.Info("raw UDP proxy booted")

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	clientManager := connection.NewUDPClientManager(ctx, logger, entry)
	defer clientManager.Cleanup()

	buffer := make([]byte, 65507) // Max UDP payload size
	for {
		n, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("listener closed due to context cancellation")
				return true, nil
			}
			logger.Error("failed to read UDP packet", zap.Error(err))
			continue
		}

		if !entry.AllowedFrom(clientAddr) {
			logger.Warn("packet rejected",
				zap.String("client", clientAddr.String()),
			)
			continue
		}

		go clientManager.HandlePacket(clientAddr, buffer[:n], conn)
	}
}
