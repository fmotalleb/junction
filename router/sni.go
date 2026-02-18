package router

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/crypto/tls"
)

const DefaultSNIPort = "443"

var (
	sniGroups     = make(map[string][]config.EntryPoint, 0)
	groupMu       sync.Mutex
	errSNIMissing = errors.New("SNI missing in ClientHello")
)

func init() {
	registerHandler(sniRouter)
	registerReset(func() {
		groupMu.Lock()
		sniGroups = make(map[string][]config.EntryPoint)
		groupMu.Unlock()
	})
}

func sniRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterSNI {
		return false, nil
	}

	// Register entry by tag if available
	if entry.Tag != nil {
		if isFirst := registerTaggedEntry(*entry.Tag, entry); !isFirst {
			return true, nil // listener already exists for this group
		}
	}

	return true, serveSNIRouter(ctx, entry)
}

func registerTaggedEntry(tag string, entry config.EntryPoint) bool {
	groupMu.Lock()
	defer groupMu.Unlock()

	group, ok := sniGroups[tag]
	if !ok {
		group = make([]config.EntryPoint, 0)
	}
	first := len(group) == 0
	group = append(group, entry)
	sniGroups[tag] = group
	return first
}

func serveSNIRouter(ctx context.Context, entry config.EntryPoint) error {
	logger := log.FromContext(ctx).Named("router.sni").
		With(zap.Any("entry", entry))

	addr := net.TCPAddrFromAddrPort(entry.Listen)
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		logger.Error("listen failed", zap.String("addr", entry.Listen.String()), zap.Error(err))
		return err
	}
	defer listener.Close()

	logger.Info("SNI router started")

	// Shutdown listener on context close
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("router exit due to context cancellation")
				return nil
			}
			logger.Warn("accept failed", zap.Error(err))
			continue
		}

		go handleClient(ctx, conn, entry, logger)
	}
}

func handleClient(ctx context.Context, conn net.Conn, entry config.EntryPoint, logger *zap.Logger) {
	serverName, buf, n, err := readSNI(conn, logger)
	if err != nil {
		_ = conn.Close()
		return
	}

	sni := string(serverName)
	l := logger.With(zap.String("sni", sni))

	if entry.Tag == nil {
		if !entry.Allowed(sni) {
			l.Warn("SNI rejected")
			_ = conn.Close()
			return
		}
		go proxyToTarget(ctx, conn, sni, buf, n, l, entry)
		return
	}

	// Tagged routing
	for _, ep := range sniGroups[*entry.Tag] {
		if ep.Allowed(sni) {
			go proxyToTarget(ctx, conn, sni, buf, n, l, ep)
			return
		}
	}

	l.Warn("no matching entry for SNI")
	_ = conn.Close()
}

// PROXY HANDLER.
func proxyToTarget(parentCtx context.Context, client net.Conn, sni string, buf []byte, n int, logger *zap.Logger, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()

	target := net.JoinHostPort(sni, entry.GetTargetOr(DefaultSNIPort))
	server, err := dialTarget(entry.Proxy, target, logger)
	if err != nil {
		_ = client.Close()
		return
	}
	defer server.Close()

	if _, err := server.Write(buf[:n]); err != nil {
		logger.Error("initial write failed", zap.Error(err))
		_ = client.Close()
		return
	}

	relayTraffic(ctx, client, server, logger)
}

func readSNI(conn net.Conn, logger *zap.Logger) ([]byte, []byte, int, error) {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		logger.Error("client read failed", zap.Error(err))
		return nil, nil, 0, err
	}

	name := tls.ExtractSNI(buf[:n])
	if name == nil {
		logger.Warn("SNI missing")
		return nil, nil, 0, errSNIMissing
	}
	return name, buf, n, nil
}
