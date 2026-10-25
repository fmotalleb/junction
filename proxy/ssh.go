package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/net/proxy"

	"github.com/fmotalleb/junction/config"
)

var ErrSSHAuthMissing = errors.New("no auth method provided (password or key path required)")

func init() {
	registerGenerator(sshDialer)
}

func sshDialer(url *url.URL, dialer proxy.Dialer) (proxy.Dialer, error) {
	if url.Scheme != "ssh" {
		return nil, nil
	}

	if url.User == nil {
		return nil, errors.New("SSH proxy URL must include a username")
	}
	user := url.User.Username()
	pass, hasPass := url.User.Password()

	var auth []gossh.AuthMethod
	switch {
	case hasPass:
		auth = append(auth, gossh.Password(pass))
	case url.Path != "":
		key, err := readKeyFile(url.Path)
		if err != nil {
			return nil, err
		}
		auth = append(auth, gossh.PublicKeys(key))
	default:
		return nil, ErrSSHAuthMissing
	}

	host := url.Host
	if url.Port() == "" {
		host = net.JoinHostPort(url.Hostname(), "22")
	}
	verifyHostKey := gossh.InsecureIgnoreHostKey()
	if config.HardenedMode() {
		knownHostsPath := url.Query().Get("known_hosts")
		if knownHostsPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("locate SSH known_hosts: %w", err)
			}
			knownHostsPath = filepath.Join(home, ".ssh", "known_hosts")
		}
		verifyHostKey, err = knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("load SSH known_hosts %q: %w", knownHostsPath, err)
		}
	}

	var sshTimeout time.Duration
	if config.HardenedMode() {
		sshTimeout = 10 * time.Second
	}
	clientConfig := &gossh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: verifyHostKey,
		Timeout:         sshTimeout,
	}

	return &sshProxyDialer{
		addr:   host,
		config: clientConfig,
		dialer: dialer,
	}, nil
}

func readKeyFile(keyPath string) (gossh.Signer, error) {
	//nolint:gosec // The key path is operator configuration, not request input.
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	return gossh.ParsePrivateKey(key)
}

type sshProxyDialer struct {
	addr   string
	config *gossh.ClientConfig
	dialer proxy.Dialer
}

func (s *sshProxyDialer) Dial(network, address string) (net.Conn, error) {
	return s.DialContext(context.Background(), network, address)
}

func (s *sshProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Establish TCP connection via parent proxy dialer
	rawConn, err := DialWith(ctx, s.dialer, "tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("proxy dial to %s failed: %w", s.addr, err)
	}

	// NewClientConn takes no context, so bound the handshake with a deadline
	// instead of letting a silent peer pin this goroutine forever.
	var deadline time.Time
	if timeout := DialTimeout(); timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	if ctxDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || ctxDeadline.Before(deadline)) {
		deadline = ctxDeadline
	}
	if !deadline.IsZero() {
		_ = rawConn.SetDeadline(deadline)
	}
	handshakeDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = rawConn.Close()
		case <-handshakeDone:
		}
	}()

	conn, chans, reqs, err := gossh.NewClientConn(rawConn, s.addr, s.config)
	close(handshakeDone)
	<-watcherDone
	if err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("SSH handshake failed: %w", err)
	}
	_ = rawConn.SetDeadline(time.Time{})

	sshClient := gossh.NewClient(conn, chans, reqs)

	// Use SSH client to open a remote connection
	remoteConn, err := sshClient.DialContext(ctx, network, address)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("SSH remote dial failed: %w", err)
	}
	return &sshTunnelConn{Conn: remoteConn, client: sshClient}, nil
}

// sshTunnelConn binds the lifetime of the whole SSH session to the tunnel
// connection returned to the caller. Without it the session, its mux goroutines
// and the socket underneath them stayed alive after the caller closed the
// tunnel, which is what made long runs exhaust memory and socket buffers.
type sshTunnelConn struct {
	net.Conn
	client *gossh.Client
	once   sync.Once
}

func (c *sshTunnelConn) Close() error {
	c.once.Do(func() {
		// Close the transport before the channel: channel.Close sends an SSH
		// message and can block forever if the peer stopped reading. Closing
		// the transport first also releases blocked mux reads and writes.
		_ = c.client.Close()
	})
	return c.Conn.Close()
}
