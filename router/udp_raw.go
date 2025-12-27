package router

import (
	"context"
	"net"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/connection"
	"go.uber.org/zap"
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
		With(zap.Any("entry", entry))

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

		go clientManager.HandlePacket(clientAddr, buffer[:n], conn)
	}
}
