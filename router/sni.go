package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/crypto/tls"
	"github.com/fmotalleb/junction/proxy"
)

const DefaultSNIPort = "443"

var (
	sniGroups     = make(map[string][]config.EntryPoint, 0)
	groupMu       sync.Mutex
	errSNIMissing = errors.New("SNI missing in ClientHello")
)

func sniRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterSNI {
		return false, nil
	}
	if _, err := proxy.NewChain(entry.Proxy); err != nil {
		return true, fmt.Errorf("invalid SNI proxy chain: %w", err)
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
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
		)

	addr := net.TCPAddrFromAddrPort(entry.Listen)
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		logger.Error("listen failed", zap.String("addr", entry.Listen.String()), zap.Error(err))
		return err
	}
	defer listener.Close()
	active := make(chan struct{}, maxConcurrentTunnels)
	var workers sync.WaitGroup
	defer func() { workers.Wait() }()

	logger.Info("SNI router started")

	// Shutdown listener on context close
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var retry acceptRetry
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("router exit due to context cancellation")
				return nil
			}
			logger.Warn("accept failed", zap.Error(err))
			if !retry.wait(ctx) {
				logger.Info("router exit due to context cancellation")
				return nil
			}
			continue
		}
		retry.reset()

		setKeepAlive(conn)
		select {
		case active <- struct{}{}:
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { <-active }()
				handleClient(ctx, conn, entry, logger)
			}()
		default:
			logger.Warn("maximum concurrent SNI tunnels reached", zap.Int("limit", maxConcurrentTunnels))
			_ = conn.Close()
		}
	}
}

func handleClient(ctx context.Context, conn net.Conn, entry config.EntryPoint, logger *zap.Logger) {
	remote := conn.RemoteAddr()
	if entry.Tag == nil {
		if !entry.AllowedFrom(remote) {
			logger.Debug("connection rejected", zap.String("client", remote.String()))
			_ = conn.Close()
			return
		}
	}

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
		proxyToTarget(ctx, conn, sni, buf, n, l, entry)
		return
	}

	// Tagged routing
	groupMu.Lock()
	group := append([]config.EntryPoint(nil), sniGroups[*entry.Tag]...)
	groupMu.Unlock()
	for _, ep := range group {
		if ep.Allowed(sni) && ep.AllowedFrom(remote) {
			proxyToTarget(ctx, conn, sni, buf, n, l, ep)
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
	defer client.Close()

	target := net.JoinHostPort(sni, entry.GetTargetOr(DefaultSNIPort))
	server, err := dialTarget(ctx, entry.Proxy, target, logger)
	if err != nil {
		return
	}
	defer server.Close()

	// Bound the buffered ClientHello replay: relayTraffic only takes over
	// afterwards, so an unbounded write here would outlive every timeout.
	_ = server.SetWriteDeadline(time.Now().Add(proxy.DefaultDialTimeout))
	if _, err := server.Write(buf[:n]); err != nil {
		logger.Error("initial write failed", zap.Error(err))
		return
	}
	_ = server.SetWriteDeadline(time.Time{})

	relayTraffic(ctx, client, server, logger)
}

const sniReadTimeout = 15 * time.Second

func readSNI(conn net.Conn, logger *zap.Logger) ([]byte, []byte, int, error) {
	// A client that connects and never sends its hello would otherwise park
	// this goroutine and its socket forever: no timeout applies this early.
	_ = conn.SetReadDeadline(time.Now().Add(sniReadTimeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	const maxClientHelloBytes = 64 << 10
	buf := make([]byte, 0, maxClientHelloBytes)
	for {
		var header [5]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return nil, nil, 0, err
		}
		recordLen := int(header[3])<<8 | int(header[4])
		if header[0] != 0x16 || recordLen == 0 || recordLen > 18432 || len(buf)+len(header)+recordLen > maxClientHelloBytes {
			return nil, nil, 0, errors.New("invalid or oversized TLS handshake record")
		}
		start := len(buf)
		buf = append(buf, header[:]...)
		buf = append(buf, make([]byte, recordLen)...)
		if _, err := io.ReadFull(conn, buf[start+len(header):]); err != nil {
			return nil, nil, 0, err
		}
		name, complete := tls.ClientHelloStatus(buf)
		if complete {
			if len(name) == 0 {
				logger.Debug("SNI missing", zap.String("client", conn.RemoteAddr().String()))
				return nil, nil, 0, errSNIMissing
			}
			return name, buf, len(buf), nil
		}
	}
}
