package router

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/fmotalleb/junction/config"
	"go.uber.org/zap"
)

func TestSNITunnelReapsRelay(t *testing.T) {
	for _, cause := range []string{"client-FIN", "client-RST", "target-FIN", "target-RST", "cancel"} {
		t.Run(cause, func(t *testing.T) {
			targetListener := listenTCP(t)
			defer targetListener.Close()
			_, port, err := net.SplitHostPort(targetListener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			client, conn := relayTCPPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			entry := config.EntryPoint{
				Routing: config.RouterSNI,
				Listen:  mustAddrPort(t, "127.0.0.1:8443"),
				Target:  port,
				Timeout: time.Minute,
			}
			registry := NewRegistry()
			if err := registry.Prepare([]config.EntryPoint{entry}); err != nil {
				t.Fatal(err)
			}
			handlerDone := make(chan struct{})
			go func() {
				handleClient(ctx, registry, 0, conn, entry, zap.NewNop())
				close(handlerDone)
			}()
			handshakeDone := make(chan struct{})
			go func() {
				// Emit a real ClientHello. The test origin never responds, so this
				// handshake must be unblocked when either side is terminated.
				tlsClient := tls.Client(client, &tls.Config{ServerName: "localhost", MinVersion: tls.VersionTLS12})
				_ = tlsClient.HandshakeContext(ctx)
				close(handshakeDone)
			}()
			t.Cleanup(func() {
				cancel()
				_ = client.Close()
				_ = conn.Close()
				waitRelay(t, handlerDone)
				waitRelay(t, handshakeDone)
			})
			_ = targetListener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
			target, err := targetListener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			_ = target.SetReadDeadline(time.Now().Add(time.Second))
			if n, err := target.Read(make([]byte, 4096)); err != nil || n == 0 {
				t.Fatalf("target did not receive the replayed ClientHello: n=%d err=%v", n, err)
			}
			_ = target.SetReadDeadline(time.Time{})
			switch cause {
			case "client-FIN":
				_ = client.Close()
			case "client-RST":
				_ = client.SetLinger(0)
				_ = client.Close()
			case "target-FIN":
				_ = target.Close()
			case "target-RST":
				_ = target.(*net.TCPConn).SetLinger(0)
				_ = target.Close()
			case "cancel":
				cancel()
			}
			waitRelay(t, handlerDone)
			waitRelay(t, handshakeDone)
			if cause == "client-FIN" || cause == "client-RST" || cause == "cancel" {
				assertRelayPeerClosed(t, target)
			}
		})
	}
}
