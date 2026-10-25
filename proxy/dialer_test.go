package proxy

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"
)

// A proxy that accepts the socket but never answers must not park the dialer:
// every such dial held a goroutine and two sockets for the lifetime of the
// process.
func TestChainDialHonorsContextOnStalledProxy(t *testing.T) {
	lc := net.ListenConfig{}
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		var held []net.Conn
		defer func() {
			for _, c := range held {
				_ = c.Close()
			}
		}()
		for {
			conn, acceptErr := l.Accept()
			if acceptErr != nil {
				return
			}
			held = append(held, conn)
		}
	}()

	chain, err := NewChain([]*url.URL{{
		Scheme: "socks5",
		Host:   l.Addr().String(),
	}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, dialErr := chain.DialContext(ctx, "tcp", "192.0.2.1:80")
		done <- dialErr
	}()

	select {
	case dialErr := <-done:
		if dialErr == nil {
			t.Fatal("expected dial to fail against a stalled proxy")
		}
		if !errors.Is(dialErr, context.DeadlineExceeded) && !errors.Is(dialErr, context.Canceled) {
			t.Logf("dial failed as expected with: %v", dialErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("dial ignored the context deadline and is still blocked")
	}
}

func TestNewDialerRejectsUnsupportedAndNilProxy(t *testing.T) {
	for _, chain := range [][]*url.URL{
		{nil},
		{{Scheme: "https", Host: "proxy.example:443"}},
		{{Scheme: "socks5", Host: "proxy.example"}},
	} {
		if _, err := NewDialer(chain); err == nil {
			t.Errorf("NewDialer(%v) succeeded; want validation error", chain)
		}
	}
}

func TestSSHProxyDialerRequiresKnownHosts(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	host := "127.0.0.1:2222"
	knownHostsPath := t.TempDir() + "/known_hosts"
	line := knownhosts.Line([]string{host}, signer.PublicKey())
	if err := os.WriteFile(knownHostsPath, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	proxyURL, err := url.Parse("ssh://user:password@" + host + "?known_hosts=" + url.QueryEscape(knownHostsPath))
	if err != nil {
		t.Fatal(err)
	}
	dialer, err := sshDialer(proxyURL, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	sshClient := dialer.(*sshProxyDialer)
	remote := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 2222}
	if err := sshClient.config.HostKeyCallback(host, remote, signer.PublicKey()); err != nil {
		t.Fatalf("trusted host key rejected: %v", err)
	}
	_, wrongKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrongSigner, err := gossh.NewSignerFromKey(wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := sshClient.config.HostKeyCallback(host, remote, wrongSigner.PublicKey()); err == nil {
		t.Fatal("unknown host key accepted")
	}
}
