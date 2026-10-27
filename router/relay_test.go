package router

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func relayTCPPair(t *testing.T) (*net.TCPConn, *net.TCPConn) {
	t.Helper()
	l := listenTCP(t)
	defer l.Close()
	peer := dialTCP(t, l.Addr().String()).(*net.TCPConn)
	conn, err := l.Accept()
	if err != nil {
		_ = peer.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close(); _ = conn.Close() })
	return peer, conn.(*net.TCPConn)
}

func startRelay(t *testing.T, ctx context.Context, src, dst net.Conn) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		relayTraffic(ctx, src, dst, zap.NewNop())
		close(done)
	}()
	t.Cleanup(func() {
		_ = src.Close()
		_ = dst.Close()
		waitRelay(t, done)
	})
	return done
}

func waitRelay(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("relay did not reap its copy workers after shutdown")
	}
}

func assertRelayPeerClosed(t *testing.T, peer net.Conn) {
	t.Helper()
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	_, err := peer.Read(make([]byte, 1))
	var netErr net.Error
	if err == nil || (errors.As(err, &netErr) && netErr.Timeout()) {
		t.Errorf("peer was not disconnected by relay: %v", err)
	}
}

func TestRelayDisconnectsBothPeers(t *testing.T) {
	for _, side := range []string{"client", "target"} {
		for _, shutdown := range []string{"FIN", "RST", "half-close", "local-close"} {
			t.Run(side+"/"+shutdown, func(t *testing.T) {
				client, src := relayTCPPair(t)
				target, dst := relayTCPPair(t)
				done := startRelay(t, context.Background(), src, dst)
				// Establish traffic in both directions before terminating one peer.
				for _, pair := range [][2]net.Conn{{client, target}, {target, client}} {
					_ = pair[0].SetWriteDeadline(time.Now().Add(time.Second))
					if _, err := pair[0].Write([]byte("ping")); err != nil {
						t.Fatal(err)
					}
					_ = pair[1].SetReadDeadline(time.Now().Add(time.Second))
					if _, err := io.ReadFull(pair[1], make([]byte, 4)); err != nil {
						t.Fatal(err)
					}
					_ = pair[0].SetDeadline(time.Time{})
					_ = pair[1].SetDeadline(time.Time{})
				}
				peer, local, other := client, src, target
				if side == "target" {
					peer, local, other = target, dst, client
				}
				switch shutdown {
				case "FIN":
					_ = peer.Close()
				case "RST":
					_ = peer.SetLinger(0)
					_ = peer.Close()
				case "half-close":
					_ = peer.CloseWrite()
				case "local-close":
					_ = local.Close()
				}
				waitRelay(t, done)
				assertRelayPeerClosed(t, other)
				if shutdown == "half-close" {
					assertRelayPeerClosed(t, peer)
				}
			})
		}
	}
}

type relayWriteSignal struct {
	net.Conn
	writing chan struct{}
	once    sync.Once
}

func (c *relayWriteSignal) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.writing) })
	return c.Conn.Write(p)
}

func TestRelayCancellationReapsBlockedWrites(t *testing.T) {
	for _, direction := range []string{"client-to-target", "target-to-client"} {
		t.Run(direction, func(t *testing.T) {
			client, src := net.Pipe()
			target, dst := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = target.Close() })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writing := make(chan struct{})
			var relaySrc, relayDst net.Conn = src, dst
			writer := client
			if direction == "client-to-target" {
				relayDst = &relayWriteSignal{Conn: dst, writing: writing}
			} else {
				relaySrc = &relayWriteSignal{Conn: src, writing: writing}
				writer = target
			}
			done := startRelay(t, ctx, relaySrc, relayDst)
			writeDone := make(chan struct{})
			go func() {
				_, _ = writer.Write(make([]byte, 64*1024))
				close(writeDone)
			}()
			select {
			case <-writing:
			case <-time.After(time.Second):
				t.Fatal("relay did not reach the blocked write")
			}
			cancel()
			waitRelay(t, done)
			waitRelay(t, writeDone)
			assertRelayPeerClosed(t, client)
			assertRelayPeerClosed(t, target)
		})
	}
}

func TestRelayCancellationReapsIdleReaders(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "already-canceled"} {
		t.Run(mode, func(t *testing.T) {
			client, src := relayTCPPair(t)
			target, dst := relayTCPPair(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer deadlineCancel()
			} else if mode == "already-canceled" {
				cancel()
			}
			done := startRelay(t, ctx, src, dst)
			if mode == "cancel" {
				cancel()
			}
			waitRelay(t, done)
			assertRelayPeerClosed(t, client)
			assertRelayPeerClosed(t, target)
		})
	}
}

type relayWriteFailure struct{ net.Conn }

func (*relayWriteFailure) Write([]byte) (int, error) {
	return 0, io.ErrShortWrite
}

func TestRelayWriteFailureClosesBothPeers(t *testing.T) {
	for _, direction := range []string{"client-to-target", "target-to-client"} {
		t.Run(direction, func(t *testing.T) {
			client, src := net.Pipe()
			target, dst := net.Pipe()
			t.Cleanup(func() { _ = client.Close(); _ = target.Close() })
			var relaySrc, relayDst net.Conn = src, dst
			writer := client
			if direction == "client-to-target" {
				relayDst = &relayWriteFailure{Conn: dst}
			} else {
				relaySrc = &relayWriteFailure{Conn: src}
				writer = target
			}
			done := startRelay(t, context.Background(), relaySrc, relayDst)
			writeDone := make(chan struct{})
			go func() {
				_, _ = writer.Write([]byte("payload"))
				close(writeDone)
			}()
			waitRelay(t, done)
			waitRelay(t, writeDone)
			assertRelayPeerClosed(t, client)
			assertRelayPeerClosed(t, target)
		})
	}
}

func TestRelayBackpressureDisconnectHonorsDeadline(t *testing.T) {
	client, src := net.Pipe()
	target, dst := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = target.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	writing := make(chan struct{})
	done := startRelay(t, ctx, src, &relayWriteSignal{Conn: dst, writing: writing})
	writeDone := make(chan struct{})
	go func() {
		_, _ = client.Write(make([]byte, 64*1024))
		close(writeDone)
	}()
	select {
	case <-writing:
	case <-time.After(time.Second):
		t.Fatal("relay did not reach the blocked write")
	}
	// The copy reading this client is now blocked writing to the target. It
	// cannot observe the client's EOF until that write finishes. The tunnel
	// deadline must still close both endpoints and reap both workers.
	_ = client.Close()
	waitRelay(t, done)
	waitRelay(t, writeDone)
	assertRelayPeerClosed(t, target)
}
