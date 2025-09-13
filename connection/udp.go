package connection

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/FMotalleb/junction/config"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
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

// func (m *UDPClientManager) handleTargetResponses(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
// 	defer client.targetConn.Close()

// 	buffer := make([]byte, 65507)
// 	clientKey := client.clientAddr.String()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		default:
// 		}

// 		// Set read timeout
// 		_ = client.targetConn.SetReadDeadline(time.Now().Add(time.Second))

// 		n, err := client.targetConn.Read(buffer)
// 		if err != nil {
// 			netErr := new(net.Error)
// 			if ok := errors.As(err, netErr); ok && (*netErr).Timeout() {
// 				continue // Continue on timeout
// 			}
// 			if ctx.Err() != nil {
// 				return // Context canceled
// 			}
// 			m.logger.Error("failed to read from target",
// 				zap.String("client", clientKey),
// 				zap.Error(err))
// 			break
// 		}

// 		// Forward response back to client
// 		_, err = serverConn.WriteToUDP(buffer[:n], client.clientAddr)
// 		if err != nil {
// 			m.logger.Error("failed to forward response to client",
// 				zap.String("client", clientKey),
// 				zap.Error(err))
// 			break
// 		}

// 		client.lastSeen = time.Now()
// 	}

// 	m.removeClient(clientKey)
// }

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

// TODO: merge into current code if its working

const (
	// Upper bound to avoid memory abuse from pathological GRO packets.
	// Tune as needed; must be >= largest legitimate coalesced size you expect.
	maxDatagramCap = 8 << 20 // 8 MiB
)

func (m *UDPClientManager) handleTargetResponses(ctx context.Context, client *UDPClientConn, serverConn *net.UDPConn) {
	defer client.targetConn.Close()

	var buf []byte
	clientKey := client.clientAddr.String()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_ = client.targetConn.SetReadDeadline(time.Now().Add(time.Second))

		// 1) Read one full UDP datagram with dynamic sizing (no loss).
		n, gsoSize, err := recvUDPFull(client.targetConn, &buf)
		if err != nil {
			// Timeout: continue quietly
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			// Context canceled
			if ctx.Err() != nil {
				return
			}
			// Any other error: stop this client cleanly (no spammy logs)
			m.logger.Error("read from target failed",
				zap.String("client", clientKey),
				zap.Error(err))
			break
		}

		// 2) Forward. If GRO coalesced (gsoSize>0), split into segments.
		if gsoSize > 0 && gsoSize < n {
			// Forward each coalesced segment separately
			for off := 0; off < n; off += gsoSize {
				end := off + gsoSize
				if end > n {
					end = n
				}
				if _, err := serverConn.WriteToUDP(buf[off:end], client.clientAddr); err != nil {
					m.logger.Error("failed to forward UDP segment to client",
						zap.String("client", clientKey),
						zap.Int("segmentLen", end-off),
						zap.Error(err))
					break
				}
			}
		} else {
			// Single datagram
			if _, err := serverConn.WriteToUDP(buf[:n], client.clientAddr); err != nil {
				m.logger.Error("failed to forward UDP datagram to client",
					zap.String("client", clientKey),
					zap.Int("len", n),
					zap.Error(err))
				break
			}
		}

		client.lastSeen = time.Now()
	}

	m.removeClient(clientKey)
}

// recvUDPFull reads exactly one UDP datagram from conn without dropping it,
// regardless of size (up to maxDatagramCap). It returns datagram length and
// the GRO/GSO segment size (if present; 0 if not coalesced).
func recvUDPFull(conn *net.UDPConn, buf *[]byte) (n int, gsoSize int, err error) {
	rc, err := conn.SyscallConn()
	if err != nil {
		return 0, 0, err
	}

	var peekLen int
	var perr error

	// Peek length (non-destructive): MSG_PEEK|MSG_TRUNC gives full datagram size.
	if err := rc.Read(func(fd uintptr) bool {
		peekLen, _, _, _, perr = unix.Recvmsg(int(fd), nil, nil, unix.MSG_PEEK|unix.MSG_TRUNC)
		// The runtime guarantees fd is ready; return true to release the poller.
		return true
	}); err != nil {
		return 0, 0, err
	}
	if perr != nil {
		// Propagate timeout correctly so caller can continue loop.
		if perr == syscall.EAGAIN || perr == syscall.EWOULDBLOCK {
			// Mirror net.Error Timeout to keep your outer logic unchanged.
			// We can't easily fabricate net.Error here; just return syscall.EAGAIN;
			// the caller checks SetReadDeadline already.
			return 0, 0, perr
		}
		return 0, 0, perr
	}
	if peekLen <= 0 {
		return 0, 0, syscall.ECONNRESET
	}
	if peekLen > maxDatagramCap {
		// Protect memory. You can choose to close the client here instead.
		return 0, 0, syscall.EMSGSIZE
	}

	// Ensure capacity
	if cap(*buf) < peekLen {
		*buf = make([]byte, peekLen)
	} else {
		*buf = (*buf)[:peekLen]
	}

	// Optional oob for GRO/SEGMENT metadata.
	oob := make([]byte, 128)
	var nn, oobn int
	var rerr error

	// Receive the full datagram (consumes it).
	if err := rc.Read(func(fd uintptr) bool {
		nn, oobn, _, _, rerr = unix.Recvmsg(int(fd), *buf, oob, 0)
		return true
	}); err != nil {
		return 0, 0, err
	}
	if rerr != nil {
		return 0, 0, rerr
	}
	if nn > peekLen {
		// Should not happen, but clamp just in case.
		nn = peekLen
	}

	seg := parseUDPGSOSegmentSize(oob[:oobn])
	return nn, seg, nil
}

// parseUDPGSOSegmentSize extracts the per-segment size from UDP GRO/GSO cmsg.
// Returns 0 if not present. On Linux/x86 this is a 16-bit value (host endian).
func parseUDPGSOSegmentSize(oob []byte) int {
	if len(oob) == 0 {
		return 0
	}
	cmsgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return 0
	}
	for _, c := range cmsgs {
		// Both appear on various kernels; use either.
		if (c.Header.Level == unix.SOL_UDP && c.Header.Type == unix.UDP_SEGMENT) ||
			(c.Header.Level == unix.SOL_UDP && c.Header.Type == unix.UDP_GRO) {
			if len(c.Data) >= 2 {
				// Linux is little-endian on common archs.
				return int(binary.LittleEndian.Uint16(c.Data[:2]))
			}
		}
	}
	return 0
}
