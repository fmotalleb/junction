package connection

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/FMotalleb/go-tools/env"
	"github.com/FMotalleb/junction/config"
	"go.uber.org/zap"
)

type UDPClientManager struct {
	ctx        context.Context
	logger     *zap.Logger
	entry      config.EntryPoint
	clients    map[string]*UDPClientConn
	clientsMux sync.RWMutex
}

type UDPClientConn struct {
	clientAddr *net.UDPAddr
	targetConn *net.UDPConn
	lastSeen   time.Time
	cancel     context.CancelFunc
}

func NewUDPClientManager(ctx context.Context, logger *zap.Logger, entry config.EntryPoint) *UDPClientManager {
	return &UDPClientManager{
		ctx:     ctx,
		logger:  logger,
		entry:   entry,
		clients: make(map[string]*UDPClientConn),
	}
}

func (m *UDPClientManager) HandlePacket(clientAddr *net.UDPAddr, data []byte, serverConn *net.UDPConn) {
	clientKey := clientAddr.String()

	m.clientsMux.RLock()
	client, exists := m.clients[clientKey]
	m.clientsMux.RUnlock()

	if !exists {
		client = m.createClientConnection(clientAddr, serverConn)
		if client == nil {
			return
		}
	}

	client.lastSeen = time.Now()

	// Forward packet to target
	_, err := client.targetConn.Write(data)
	if err != nil {
		m.logger.Error("failed to forward packet to target",
			zap.String("client", clientKey),
			zap.Error(err))
		m.removeClient(clientKey)
	}
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
		lastSeen:   time.Now(),
		cancel:     cancel,
	}

	m.clientsMux.Lock()
	m.clients[clientKey] = client
	m.clientsMux.Unlock()

	m.logger.Debug("created new UDP client connection", zap.String("client", clientKey))

	// Start goroutine to handle responses from target
	go m.handleTargetResponses(ctx, client, serverConn)

	// Start cleanup timer for this client
	go m.clientCleanupTimer(ctx, clientKey)

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
		m.logger.Error("failed to connect to target",
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

		client.lastSeen = time.Now()
	}

	m.removeClient(clientKey)
}

func (m *UDPClientManager) clientCleanupTimer(ctx context.Context, clientKey string) {
	timeout := m.entry.GetTimeout()
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout
	}

	ticker := time.NewTicker(timeout / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.clientsMux.RLock()
			client, exists := m.clients[clientKey]
			m.clientsMux.RUnlock()

			if !exists {
				return
			}

			if time.Since(client.lastSeen) > timeout {
				m.logger.Debug("cleaning up idle UDP client", zap.String("client", clientKey))
				m.removeClient(clientKey)
				return
			}
		}
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
