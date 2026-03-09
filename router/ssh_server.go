package router

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"

	"github.com/fmotalleb/junction/config"
)

const (
	sshParamHostKeyPath       = "host_key"
	sshParamAuthorizedKeys    = "authorized_keys"
	sshParamAuthorizedKeysAny = "allow-all"
	sshParamPassword          = "password"
	sshParamUser              = "user"
	sshParamUsers             = "users"
	sshParamBanner            = "banner"
	sshParamAllowAnyUser      = "allow_any_user"
	sshParamAllowAnyPublicKey = "allow_any_public_key"
)

func init() {
	registerHandler(sshServerRouter)
}

func sshServerRouter(ctx context.Context, entry config.EntryPoint) (bool, error) {
	if entry.Routing != config.RouterSSHServer {
		return false, nil
	}

	logger := log.FromContext(ctx).
		Named("router.ssh-server").
		With(
			zap.String("router", string(entry.Routing)),
			zap.String("listen", entry.Listen.String()),
		)

	if entry.Params == nil {
		logger.Error("missing params for ssh-server")
		return true, buildFieldMissing("ssh-server", "params")
	}

	serverConfig, err := buildSSHServerConfig(entry.Params, logger)
	if err != nil {
		return true, err
	}

	addrPort := entry.Listen
	tcpAddr := net.TCPAddrFromAddrPort(addrPort)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return true, err
	}
	defer listener.Close()

	logger.Info("SSH jump server booted")

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("listener closed due to context cancellation")
				return true, nil
			}
			logger.Error("failed to accept connection", zap.Error(err))
			continue
		}

		if !entry.AllowedFrom(conn.RemoteAddr()) {
			logger.Debug("connection rejected",
				zap.String("client", conn.RemoteAddr().String()),
			)
			_ = conn.Close()
			continue
		}

		go handleSSHConnection(ctx, logger, conn, serverConfig, entry)
	}
}

func handleSSHConnection(parentCtx context.Context, logger *zap.Logger, conn net.Conn, config *gossh.ServerConfig, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	sshConn, chans, reqs, err := gossh.NewServerConn(conn, config)
	if err != nil {
		logger.Debug("ssh handshake failed", zap.Error(err))
		return
	}
	defer sshConn.Close()

	go gossh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "direct-tcpip" {
			_ = newChannel.Reject(gossh.UnknownChannelType, "only direct-tcpip is supported")
			continue
		}

		var req directTCPIP
		if err := gossh.Unmarshal(newChannel.ExtraData(), &req); err != nil {
			_ = newChannel.Reject(gossh.Prohibited, "invalid direct-tcpip request")
			continue
		}

		destHost := req.DestAddr
		destAddr := net.JoinHostPort(req.DestAddr, strconv.Itoa(int(req.DestPort)))
		if !allowedTarget(entry, destHost, destAddr) {
			_ = newChannel.Reject(gossh.Prohibited, "destination not allowed")
			continue
		}

		target := destAddr
		targetConn, err := dialTarget(entry.Proxy, target, logger)
		if err != nil {
			_ = newChannel.Reject(gossh.ConnectionFailed, "failed to connect to destination")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			_ = targetConn.Close()
			continue
		}

		go gossh.DiscardRequests(requests)
		go relaySSH(ctx, channel, targetConn, logger)
	}
}

type directTCPIP struct {
	DestAddr   string
	DestPort   uint32
	OriginAddr string
	OriginPort uint32
}

func relaySSH(ctx context.Context, ch gossh.Channel, target net.Conn, logger *zap.Logger) {
	defer func() {
		_ = ch.Close()
		_ = target.Close()
	}()

	go func() {
		<-ctx.Done()
		_ = ch.Close()
		_ = target.Close()
	}()

	errs, _ := errgroup.WithContext(ctx)
	errs.Go(func() error {
		_, err := io.Copy(target, ch)
		return err
	})
	errs.Go(func() error {
		_, err := io.Copy(ch, target)
		return err
	})

	if err := errs.Wait(); err != nil && !errors.Is(err, net.ErrClosed) {
		logger.Debug("ssh channel closed", zap.Error(err))
	}
}

func buildSSHServerConfig(params map[string]string, logger *zap.Logger) (*gossh.ServerConfig, error) {
	hostKeyPath := strings.TrimSpace(params[sshParamHostKeyPath])
	signer, err := loadHostKey(hostKeyPath)
	if err != nil {
		return nil, err
	}

	allowedUsers := parseUsers(params)
	allowAnyUser := parseBool(params[sshParamAllowAnyUser])
	if allowAnyUser {
		allowedUsers = nil
	}

	authorizedKeysPath := strings.TrimSpace(params[sshParamAuthorizedKeys])
	allowAnyPublicKey := parseBool(params[sshParamAllowAnyPublicKey])
	authorizedKeys, allowAllFromKeys, err := loadAuthorizedKeys(authorizedKeysPath)
	if err != nil {
		return nil, err
	}

	if allowAllFromKeys {
		allowAnyPublicKey = true
	}
	password := params[sshParamPassword]

	if len(authorizedKeys) == 0 && password == "" && !allowAnyPublicKey {
		return nil, errors.New("ssh-server requires authorized_keys or password (or allow_any_public_key)")
	}

	config := &gossh.ServerConfig{
		BannerCallback: func(_ gossh.ConnMetadata) string {
			return params[sshParamBanner]
		},
		PublicKeyCallback: func(c gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if !userAllowed(c.User(), allowedUsers) {
				return nil, errors.New("user not allowed")
			}
			if allowAnyPublicKey {
				return nil, nil
			}
			if _, ok := authorizedKeys[string(key.Marshal())]; ok {
				return nil, nil
			}
			return nil, errors.New("unknown public key")
		},
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			if password == "" {
				return nil, errors.New("password auth disabled")
			}
			if !userAllowed(c.User(), allowedUsers) {
				return nil, errors.New("user not allowed")
			}
			if string(pass) != password {
				return nil, errors.New("invalid password")
			}
			return nil, nil
		},
	}
	config.AddHostKey(signer)
	return config, nil
}

func parseUsers(params map[string]string) map[string]struct{} {
	users := make(map[string]struct{})
	addUser := func(raw string) {
		name := strings.TrimSpace(raw)
		if name != "" {
			users[name] = struct{}{}
		}
	}

	if user := params[sshParamUser]; user != "" {
		addUser(user)
	}
	if raw := params[sshParamUsers]; raw != "" {
		for _, u := range strings.Split(raw, ",") {
			addUser(u)
		}
	}

	if len(users) == 0 {
		return nil
	}
	return users
}

func userAllowed(user string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	_, ok := allowed[user]
	return ok
}

func loadAuthorizedKeys(path string) (map[string]struct{}, bool, error) {
	if path == "" {
		return map[string]struct{}{}, false, nil
	}
	if strings.EqualFold(strings.TrimSpace(path), sshParamAuthorizedKeysAny) {
		return map[string]struct{}{}, true, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, errors.Join(fmt.Errorf("failed to read authorized_keys: %s", path), err)
	}
	keys := make(map[string]struct{})
	for len(data) > 0 {
		pub, _, _, rest, err := gossh.ParseAuthorizedKey(data)
		if err != nil {
			return nil, false, errors.Join(errors.New("failed to parse authorized_keys"), err)
		}
		keys[string(pub.Marshal())] = struct{}{}
		data = rest
	}
	return keys, false, nil
}

func loadHostKey(path string) (gossh.Signer, error) {
	if strings.TrimSpace(path) == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, errors.Join(errors.New("failed to generate host key"), err)
		}
		signer, err := gossh.NewSignerFromKey(priv)
		if err != nil {
			return nil, errors.Join(errors.New("failed to create host key signer"), err)
		}
		return signer, nil
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("failed to read host key: %s", path), err)
	}
	signer, err := gossh.ParsePrivateKey(key)
	if err != nil {
		return nil, errors.Join(errors.New("failed to parse host key"), err)
	}
	return signer, nil
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
