package connection

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fmotalleb/go-tools/env"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

type UDPClientManager struct {
	ctx        context.Context
	logger     *zap.Logger
	entry      config.EntryPoint
	clients    map[string]*UDPClientConn
	clientsMux sync.RWMutex
	createMu   sync.Mutex
}

type UDPClientConn struct {
	clientAddr *net.UDPAddr
	targetConn *net.UDPConn
	lastSeen   atomic.Int64 // unix nanos, updated concurrently by reader/writer
	cancel     context.CancelFunc
}

// NewUDPClientManager builds the per-entrypoint client table and starts the
// single sweeper goroutine that retires idle clients. Idle detection is done
// centrally instead of with one timer goroutine per client, so a busy entrypoint
// does not grow a timer for every address it has ever seen.
func NewUDPClientManager(ctx context.Context, logger *zap.Logger, entry config.EntryPoint) *UDPClientManager {
	m := &UDPClientManager{
		ctx:     ctx,
		logger:  logger,
		entry:   entry,
		clients: make(map[string]*UDPClientConn),
	}
	go m.sweepIdleClients(ctx)
	return m
}

func (m *UDPClientManager) HandlePacket(clientAddr *net.UDPAddr, data []byte, serverConn *net.UDPConn) {
	clientKey := clientAddr.String()

	client := m.lookupOrCreate(clientKey, clientAddr, serverConn)
	if client == nil {
		return
	}

	client.lastSeen.Store(time.Now().UnixNano())

	// Forward packet to target
	_, err := client.targetConn.Write(data)
	if err != nil {
		m.logger.Error("failed to forward packet to target",
			zap.String("client", clientKey),
			zap.Error(err))
		m.removeClient(clientKey)
	}
}

// lookupOrCreate returns the client connection for clientKey, dialing a new one
// when needed. Creation is serialized: two packets from an unseen client used
// to both dial, and the loser of the map write was orphaned together with its
// socket and reader goroutine.
func (m *UDPClientManager) lookupOrCreate(clientKey string, clientAddr *net.UDPAddr, serverConn *net.UDPConn) *UDPClientConn {
	m.clientsMux.RLock()
	client, exists := m.clients[clientKey]
	m.clientsMux.RUnlock()
	if exists {
		return client
	}

	m.createMu.Lock()
	defer m.createMu.Unlock()

	m.clientsMux.RLock()
	client, exists = m.clients[clientKey]
	m.clientsMux.RUnlock()
	if exists {
		return client
	}

	return m.createClientConnection(clientAddr, serverConn)
}

func (m *UDPClientManager) Cleanup() {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	for clientKey, client := range m.clients {
		client.cancel()
		delete(m.clients, clientKey)
	}
}

func (m *UDPClientManager) createClientConnection(clientAddr *net.UDPAddr, serverConn *net.UDPConn) *UDPClientConn {
	clientKey := clientAddr.String()

	// Create connection to target
	targetConn, err := m.dialTarget()
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	client := &UDPClientConn{
		clientAddr: clientAddr,
		targetConn: targetConn,
		cancel:     cancel,
	}
	client.lastSeen.Store(time.Now().UnixNano())

	m.clientsMux.Lock()
	m.clients[clientKey] = client
	m.clientsMux.Unlock()

	m.logger.Debug("created new UDP client connection", zap.String("client", clientKey))

	// One reader per socket; idle retirement is handled by the manager's sweeper.
	go m.handleTargetResponses(ctx, client, serverConn)

	return client
}

func (m *UDPClientManager) dialTarget() (*net.UDPConn, error) {
	targetAddr, err := net.ResolveUDPAddr("udp", m.entry.Target)
	if err != nil {
		m.logger.Error("failed to resolve target address",
			zap.String("target", m.entry.Target),
			zap.Error(err))
		return nil, err
	}

	// Handle proxy if configured
	if len(m.entry.Proxy) > 0 {
		// Note: UDP proxy support would need to be implemented in DialTarget
		// For now, direct connection
		m.logger.Warn("UDP proxy not fully implemented, using direct connection")
	}

	targetConn, err := net.DialUDP("udp", nil, targetAddr)
	if err != nil {
		m.logger.Debug("failed to connect to target",
			zap.String("target", m.entry.Target),
			zap.Error(err))
		return nil, err
	}

	return targetConn, nil
}

var bufferSize = sync.OnceValue(
	func() int { return env.IntOr("UDP_BUFFER", 65507) },
)

func (m *UDPClientManager) handleTargetResponses(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
	defer client.targetConn.Close()
	size := bufferSize()
	buffer := make([]byte, size)
	clientKey := client.clientAddr.String()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read timeout
		_ = client.targetConn.SetReadDeadline(time.Now().Add(time.Second))

		n, err := client.targetConn.Read(buffer)
		if err != nil {
			netErr := new(net.Error)
			if ok := errors.As(err, netErr); ok && (*netErr).Timeout() {
				continue // Continue on timeout
			}
			if ctx.Err() != nil {
				return // Context canceled
			}
			m.logger.Error("failed to read from target",
				zap.String("client", clientKey),
				zap.Int("buffer-size", size),
				zap.Error(err))
			break
		}

		// Forward response back to client
		_, err = serverConn.WriteToUDP(buffer[:n], client.clientAddr)
		if err != nil {
			m.logger.Error("failed to forward response to client",
				zap.String("client", clientKey),
				zap.Error(err))
			break
		}

		client.lastSeen.Store(time.Now().UnixNano())
	}

	m.removeClient(clientKey)
}

// idleTimeout is how long a client may stay silent before it is retired.
func (m *UDPClientManager) idleTimeout() time.Duration {
	timeout := m.entry.GetTimeout()
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout
	}
	return timeout
}

// sweepIdleClients is the single timer for the whole manager: it wakes up twice
// per idle timeout and retires every client that has been silent that long.
func (m *UDPClientManager) sweepIdleClients(ctx context.Context) {
	timeout := m.idleTimeout()
	interval := timeout / 2
	if interval <= 0 {
		interval = time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweep(timeout)
		}
	}
}

// sweep retires every client whose last packet is older than timeout.
func (m *UDPClientManager) sweep(timeout time.Duration) {
	cutoff := time.Now().Add(-timeout).UnixNano()

	m.clientsMux.RLock()
	var idle []string
	for clientKey, client := range m.clients {
		if client.lastSeen.Load() < cutoff {
			idle = append(idle, clientKey)
		}
	}
	m.clientsMux.RUnlock()

	for _, clientKey := range idle {
		m.logger.Debug("cleaning up idle UDP client", zap.String("client", clientKey))
		m.removeClient(clientKey)
	}
}

func (m *UDPClientManager) removeClient(clientKey string) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	if client, exists := m.clients[clientKey]; exists {
		client.cancel()
		delete(m.clients, clientKey)
	}
}
