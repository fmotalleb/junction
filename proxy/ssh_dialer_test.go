package proxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

type sshServer struct {
	l          net.Listener
	openConns  atomic.Int64
	accepted   atomic.Int64
	closeCh    chan struct{}
	closeOnce  sync.Once
	serverConf *gossh.ServerConfig
}

func newSSHServer(t *testing.T) *sshServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	conf := &gossh.ServerConfig{NoClientAuth: true}
	conf.AddHostKey(signer)

	l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &sshServer{l: l, closeCh: make(chan struct{}), serverConf: conf}
	go s.serve()
	return s
}

func (s *sshServer) serve() {
	for {
		c, err := s.l.Accept()
		if err != nil {
			return
		}
		s.accepted.Add(1)
		s.openConns.Add(1)
		go func(c net.Conn) {
			defer func() {
				_ = c.Close()
				s.openConns.Add(-1)
			}()
			_, chans, reqs, err := gossh.NewServerConn(c, s.serverConf)
			if err != nil {
				return
			}
			go gossh.DiscardRequests(reqs)
			for newChan := range chans {
				if newChan.ChannelType() != "direct-tcpip" {
					_ = newChan.Reject(gossh.UnknownChannelType, "no")
					continue
				}
				ch, chReqs, err := newChan.Accept()
				if err != nil {
					continue
				}
				go gossh.DiscardRequests(chReqs)
				go func() {
					_, _ = io.Copy(io.Discard, ch)
					_ = ch.Close()
				}()
			}
		}(c)
	}
}

func (s *sshServer) stop() {
	s.closeOnce.Do(func() { _ = s.l.Close() })
}

func TestSSHProxyDialerLeaksConnections(t *testing.T) {
	srv := newSSHServer(t)
	defer srv.stop()

	// dummy target: accept anything
	target, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()

	d := &sshProxyDialer{
		addr: srv.l.Addr().String(),
		//nolint:gosec // Fixture: the in-process SSH server has no real host key.
		config: &gossh.ClientConfig{User: "u", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second},
		dialer: proxy.Direct,
	}

	base := runtime.NumGoroutine()

	const n = 20
	for i := 0; i < n; i++ {
		c, err := d.Dial("tcp", target.Addr().String())
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		_, _ = c.Write([]byte("ping"))
		_ = c.Close() // caller is done with this connection
	}

	time.Sleep(2 * time.Second)
	runtime.GC()
	after := runtime.NumGoroutine()
	open := srv.openConns.Load()

	t.Logf("accepted=%d still-open-at-server=%d goroutines base=%d after=%d", srv.accepted.Load(), open, base, after)

	if open > 0 {
		t.Errorf("SSH server still holds %d/%d client connections after callers closed them", open, n)
	}
	if after > base+n { // roughly >=1 goroutine per leaked ssh client
		buf := make([]byte, 1<<21)
		sz := runtime.Stack(buf, true)
		t.Errorf("goroutine leak: base=%d after=%d\n%s", base, after, buf[:sz])
	}
}
