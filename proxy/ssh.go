package proxy

// Target format
// ssh://
// 127.0.0.1:1080
// ?user=root&
// password=password&
// private_key=(base64 url version)&
// private_key_path=keypath&
// private_key_passphrase=phrase&
// host_key=aG9zdGtleQ&
// host_key_algorithms=YWxnbw&
// client_version=version

import (
	"bytes"
	"encoding/base64"
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

const (
	keyFileQuery       = "key_file"
	keyContentQuery    = "key_content"
	keyPassphraseQuery = "passphrase"
)

func init() {
	registerGenerator(sshDialer)
}

type sshProxyDialer struct {
	addr   string
	config *gossh.ClientConfig
	dialer proxy.Dialer
}

func sshDialer(url *url.URL, dialer proxy.Dialer) (proxy.Dialer, error) {
	if url.Scheme != "ssh" {
		return nil, nil
	}

	user := url.User.Username()
	pass, hasPass := url.User.Password()

	var auth []gossh.AuthMethod
	query := url.Query()
	keyPass := ""
	if query.Has(keyPassphraseQuery) {
		keyPass = query.Get(keyPassphraseQuery)
	}
	switch {
	case hasPass:
		auth = append(auth, gossh.Password(pass))
	case query.Has(keyFileQuery):
		keyPath := query.Get(keyFileQuery)
		key, err := readKeyFile(keyPath, keyPass)
		if err != nil {
			return nil, err
		}
		auth = append(auth, gossh.PublicKeys(key))
	case query.Has(keyContentQuery):
		keyContent := query.Get(keyContentQuery)
		key, err := readB64Key(keyContent, keyPass)
		if err != nil {
			return nil, err
		}
		auth = append(auth, gossh.PublicKeys(key))
	default:
		return nil, errors.New("no auth method provided (password or key path required)")
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

func readKeyFile(keyPath, passphrase string) (gossh.Signer, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	return parseKey(key, []byte(passphrase))
}

func readB64Key(keyPath, passphrase string) (gossh.Signer, error) {
	r := bytes.NewBufferString(keyPath)
	k := make([]byte, 0)
	d := base64.NewDecoder(base64.StdEncoding, r)
	_, err := d.Read(k)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	return parseKey(k, []byte(passphrase))
}

func parseKey(key, passphrase []byte) (gossh.Signer, error) {
	if len(passphrase) != 0 {
		return gossh.ParsePrivateKeyWithPassphrase(key, passphrase)
	}
	return gossh.ParsePrivateKey(key)
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
