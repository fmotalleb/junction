package connection

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

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
	closing    bool
	readers    sync.WaitGroup
	workers    sync.WaitGroup
	cancel     context.CancelFunc
	bufferSize int
}

const (
	maxUDPClientSessions          = 4096
	maxUDPClientSessionsPerSource = 64
)

const (
	defaultUDPBufferSize = 65507
	maxUDPBufferSize     = 65507
)

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
func NewUDPClientManager(ctx context.Context, logger *zap.Logger, entry config.EntryPoint) (*UDPClientManager, error) {
	bufferSize := defaultUDPBufferSize
	if config.HardenedMode() {
		var err error
		bufferSize, err = udpBufferSize(os.Getenv("UDP_BUFFER"))
		if err != nil {
			return nil, err
		} else if raw := os.Getenv("UDP_BUFFER"); raw == "" {
			bufferSize = defaultUDPBufferSize
		}
	} else if size, err := strconv.Atoi(os.Getenv("UDP_BUFFER")); err == nil && size > 0 {
		bufferSize = size
	}
	managerCtx, cancel := context.WithCancel(ctx)
	m := &UDPClientManager{
		ctx:        managerCtx,
		logger:     logger,
		entry:      entry,
		clients:    make(map[string]*UDPClientConn),
		cancel:     cancel,
		bufferSize: bufferSize,
	}
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		m.sweepIdleClients(managerCtx)
	}()
	return m, nil
}

func (m *UDPClientManager) HandlePacket(clientAddr *net.UDPAddr, data []byte, serverConn *net.UDPConn) {
	clientKey := clientAddr.String()

	client := m.lookupOrCreate(clientKey, clientAddr, serverConn)
	if client == nil {
		return
	}

	client.lastSeen.Store(time.Now().UnixNano())

	// Forward packet to target
	_ = client.targetConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err := client.targetConn.Write(data)
	if err != nil {
		m.logger.Error("failed to forward packet to target",
			zap.String("client", clientKey),
			zap.Error(err))
		m.removeClient(clientKey, client)
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
	if m.closing {
		return nil
	}

	m.clientsMux.RLock()
	client, exists = m.clients[clientKey]
	m.clientsMux.RUnlock()
	if exists {
		return client
	}

	return m.createClientConnection(clientAddr, serverConn)
}

func (m *UDPClientManager) Cleanup() {
	// Cancel first so an in-progress context-aware resolution/dial releases
	// createMu and lets cleanup proceed.
	m.cancel()
	m.createMu.Lock()
	m.closing = true
	m.clientsMux.Lock()
	for clientKey, client := range m.clients {
		client.cancel()
		_ = client.targetConn.Close()
		delete(m.clients, clientKey)
	}
	m.clientsMux.Unlock()
	m.createMu.Unlock()
	m.readers.Wait()
	m.workers.Wait()
}

func (m *UDPClientManager) createClientConnection(clientAddr *net.UDPAddr, serverConn *net.UDPConn) *UDPClientConn {
	clientKey := clientAddr.String()
	if config.HardenedMode() {
		m.clientsMux.RLock()
		full := len(m.clients) >= maxUDPClientSessions
		sourceSessions := 0
		for _, client := range m.clients {
			if client.clientAddr.IP.Equal(clientAddr.IP) {
				sourceSessions++
			}
		}
		m.clientsMux.RUnlock()
		if full {
			m.logger.Debug("UDP client session limit reached", zap.Int("limit", maxUDPClientSessions))
			return nil
		}
		if sourceSessions >= maxUDPClientSessionsPerSource {
			m.logger.Debug("UDP per-source session limit reached", zap.Int("limit", maxUDPClientSessionsPerSource))
			return nil
		}
	}

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
	m.readers.Add(1)
	go m.handleTargetResponses(ctx, client, serverConn)

	return client
}

func (m *UDPClientManager) dialTarget() (*net.UDPConn, error) {
	if len(m.entry.Proxy) > 0 {
		if config.HardenedMode() {
			return nil, errors.New("udp-raw does not support proxy chains")
		}
		m.logger.Warn("UDP proxying is not implemented; dialing target directly")
	}
	dialCtx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "udp", m.entry.Target)
	if err != nil {
		m.logger.Debug("failed to resolve or connect to target",
			zap.String("target", m.entry.Target),
			zap.Error(err))
		return nil, err
	}
	targetConn, ok := conn.(*net.UDPConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("UDP dial returned a non-UDP connection")
	}
	return targetConn, nil
}

func udpBufferSize(raw string) (int, error) {
	if raw == "" {
		return defaultUDPBufferSize, nil
	}
	size, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("UDP_BUFFER must be an integer")
	}
	if size < 1 || size > maxUDPBufferSize {
		return 0, errors.New("UDP_BUFFER must be between 1 and 65507 bytes")
	}
	return size, nil
}

func (m *UDPClientManager) handleTargetResponses(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
	defer m.readers.Done()
	defer client.targetConn.Close()
	size := m.bufferSize
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

	m.removeClient(clientKey, client)
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
	var idle []struct {
		key    string
		client *UDPClientConn
	}
	for clientKey, client := range m.clients {
		if client.lastSeen.Load() < cutoff {
			idle = append(idle, struct {
				key    string
				client *UDPClientConn
			}{clientKey, client})
		}
	}
	m.clientsMux.RUnlock()

	for _, candidate := range idle {
		m.logger.Debug("cleaning up idle UDP client", zap.String("client", candidate.key))
		m.removeIdleClient(candidate.key, candidate.client, time.Now().Add(-timeout).UnixNano())
	}
}

func (m *UDPClientManager) removeIdleClient(clientKey string, expected *UDPClientConn, cutoff int64) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()
	if client, exists := m.clients[clientKey]; exists && client == expected && client.lastSeen.Load() < cutoff {
		client.cancel()
		_ = client.targetConn.Close()
		delete(m.clients, clientKey)
	}
}

func (m *UDPClientManager) removeClient(clientKey string, expected *UDPClientConn) {
	m.clientsMux.Lock()
	defer m.clientsMux.Unlock()

	if client, exists := m.clients[clientKey]; exists && client == expected {
		client.cancel()
		_ = client.targetConn.Close()
		delete(m.clients, clientKey)
	}
}
