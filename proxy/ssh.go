package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

var ErrSSHAuthMissing = errors.New("no auth method provided (password or key path required)")

func init() {
	registerGenerator(sshDialer)
}

func sshDialer(url *url.URL, dialer proxy.Dialer) (proxy.Dialer, error) {
	if url.Scheme != "ssh" {
		return nil, nil
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
	if !strings.Contains(host, ":") {
		host += ":22"
	}

	clientConfig := &gossh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ignoreHostKey,
		Timeout:         10 * time.Second,
	}

	return &sshProxyDialer{
		addr:   host,
		config: clientConfig,
		dialer: dialer,
	}, nil
}

func readKeyFile(keyPath string) (gossh.Signer, error) {
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
	// Establish TCP connection via parent proxy dialer
	rawConn, err := s.dialer.Dial("tcp", s.addr)
	if err != nil {
		return nil, fmt.Errorf("proxy dial to %s failed: %w", s.addr, err)
	}

	// Perform SSH handshake over rawConn
	conn, chans, reqs, err := gossh.NewClientConn(rawConn, s.addr, s.config)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("SSH handshake failed: %w", err)
	}

	sshClient := gossh.NewClient(conn, chans, reqs)

	// Use SSH client to open a remote connection
	remoteConn, err := sshClient.Dial(network, address)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("SSH remote dial failed: %w", err)
	}
	return remoteConn, nil
}

func ignoreHostKey(_ string, _ net.Addr, _ gossh.PublicKey) error {
	return nil
}
