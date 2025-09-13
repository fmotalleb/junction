package connection

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

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

func (m *UDPClientManager) handleTargetResponses(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
	defer client.targetConn.Close()

	// Use a larger initial buffer that can handle most common cases
	buffer := make([]byte, 65535) // Max UDP packet size
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
				zap.Error(err))
			break
		}

		// Handle the received data
		if err := m.forwardResponse(buffer[:n], client, serverConn); err != nil {
			m.logger.Error("failed to forward response",
				zap.String("client", clientKey),
				zap.Error(err))
			break
		}

		client.lastSeen = time.Now()
	}

	m.removeClient(clientKey)
}

func (m *UDPClientManager) forwardResponse(data []byte, client *UDPClientConn, serverConn *net.UDPConn) error {
	// For very large messages, you might want to implement fragmentation handling
	// Here's a simple approach that handles sending large UDP packets

	maxUDPSize := 65507 // Maximum UDP payload size

	if len(data) <= maxUDPSize {
		// Single packet fits within UDP limits
		_, err := serverConn.WriteToUDP(data, client.clientAddr)
		return err
	}

	// Handle large messages by implementing application-level fragmentation
	// This is a simple example - you might want more sophisticated fragmentation protocol
	m.logger.Warn("large UDP message detected, implementing simple fragmentation",
		zap.String("client", client.clientAddr.String()),
		zap.Int("total_size", len(data)))

	// Split into chunks that fit within UDP limits
	chunkSize := maxUDPSize - 100 // Leave some room for headers if needed
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}

		chunk := data[i:end]
		_, err := serverConn.WriteToUDP(chunk, client.clientAddr)
		if err != nil {
			return fmt.Errorf("failed to send chunk %d: %w", i/chunkSize, err)
		}

		// Small delay to avoid overwhelming the network
		time.Sleep(1 * time.Millisecond)
	}

	return nil
}

// Alternative: Use a dynamically growing buffer for extremely large messages.
func (m *UDPClientManager) handleTargetResponsesWithDynamicBuffer(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
	defer client.targetConn.Close()

	clientKey := client.clientAddr.String()
	var reassemblyBuffer []byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read timeout
		_ = client.targetConn.SetReadDeadline(time.Now().Add(time.Second))

		// Read with a reasonable buffer size
		buffer := make([]byte, 65535)
		n, err := client.targetConn.Read(buffer)
		if err != nil {
			netErr := new(net.Error)
			if ok := errors.As(err, netErr); ok && (*netErr).Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			m.logger.Error("failed to read from target", zap.String("client", clientKey), zap.Error(err))
			break
		}

		// Process the received data
		reassemblyBuffer = append(reassemblyBuffer, buffer[:n]...)

		// Check if we have a complete message (you'll need to define what constitutes a complete message)
		if m.isCompleteMessage(reassemblyBuffer) {
			if err := m.forwardResponse(reassemblyBuffer, client, serverConn); err != nil {
				m.logger.Error("failed to forward response", zap.String("client", clientKey), zap.Error(err))
				break
			}
			reassemblyBuffer = nil // Reset buffer
		}

		client.lastSeen = time.Now()
	}

	m.removeClient(clientKey)
}

func (m *UDPClientManager) isCompleteMessage(data []byte) bool {
	// Implement your message completion logic here
	// This could be based on a specific protocol, message delimiter, length prefix, etc.
	// For example, if your protocol uses a length prefix:
	if len(data) >= 4 {
		expectedLength := binary.BigEndian.Uint32(data[:4])
		return len(data) >= int(expectedLength)+4
	}
	return false
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
