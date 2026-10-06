package connection

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
)

func clientCount(m *UDPClientManager) int {
	m.clientsMux.RLock()
	defer m.clientsMux.RUnlock()
	return len(m.clients)
}

func waitForClients(t *testing.T, m *UDPClientManager, want int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if clientCount(m) == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return clientCount(m) == want
}

// newTestManager wires a manager to a local UDP target and returns it together
// with the sockets the packet path needs.
func newTestManager(t *testing.T, timeout time.Duration) (*UDPClientManager, *net.UDPConn, context.CancelFunc) {
	t.Helper()
	target, err := (&net.ListenConfig{}).ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverConn.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	entry := config.EntryPoint{
		Target:  target.LocalAddr().String(),
		Timeout: timeout,
	}
	return NewUDPClientManager(ctx, zap.NewNop(), entry), serverConn, cancel
}

var testClient = &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41234}

func TestSweepRetiresIdleClients(t *testing.T) {
	m, serverConn, cancel := newTestManager(t, time.Hour)
	defer cancel()

	m.HandlePacket(testClient, []byte("hello"), serverConn)
	assert.Equal(t, 1, clientCount(m))

	m.clientsMux.Lock()
	for _, client := range m.clients {
		client.lastSeen.Store(time.Now().Add(-2 * time.Hour).UnixNano())
	}
	m.clientsMux.Unlock()

	m.sweep(m.idleTimeout())
	assert.Equal(t, 0, clientCount(m), "an idle client must be retired")
}

func TestSweepKeepsActiveClients(t *testing.T) {
	m, serverConn, cancel := newTestManager(t, time.Hour)
	defer cancel()

	m.HandlePacket(testClient, []byte("hello"), serverConn)
	assert.Equal(t, 1, clientCount(m))

	m.sweep(m.idleTimeout())
	assert.Equal(t, 1, clientCount(m), "a client that just sent a packet must survive")

	m.Cleanup()
	assert.Equal(t, 0, clientCount(m))
}

// TestSweeperRetiresIdleClientsOnItsOwn replaces the per-client timer goroutines:
// one manager level ticker has to notice the silence without any per-client help.
func TestSweeperRetiresIdleClientsOnItsOwn(t *testing.T) {
	m, serverConn, cancel := newTestManager(t, 400*time.Millisecond)
	defer cancel()

	m.HandlePacket(testClient, []byte("hello"), serverConn)
	assert.Equal(t, 1, clientCount(m))

	if !waitForClients(t, m, 0, 5*time.Second) {
		t.Fatalf("sweeper did not retire the idle client, %d left", clientCount(m))
	}
}
