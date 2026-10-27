package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/fmotalleb/junction/proxy"
)

func dialTarget(ctx context.Context, proxyAddr []*url.URL, target string, logger *zap.Logger) (net.Conn, error) {
	chain, err := proxy.NewChain(proxyAddr)
	if err != nil {
		logger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return nil, err
	}

	// The context bounds the connect and the proxy handshake; a silent upstream
	// otherwise left this goroutine and both sockets parked indefinitely.
	dialCtx, cancel := context.WithTimeout(ctx, proxy.DefaultDialTimeout)
	defer cancel()

	conn, err := chain.DialContext(dialCtx, "tcp", target)
	if err != nil {
		logger.Debug("failed to connect to target", zap.Error(err))
		return nil, err
	}
	setKeepAlive(conn)
	return conn, nil
}

type keepAliveConn interface {
	SetKeepAlive(bool) error
	SetKeepAlivePeriod(time.Duration) error
}

const keepAlivePeriod = 30 * time.Second

// setKeepAlive lets the OS detect peers that vanished without a FIN/RST. Such
// peers otherwise leave the relay blocked until the entry timeout expires.
func setKeepAlive(conn net.Conn) {
	if c, ok := conn.(keepAliveConn); ok {
		_ = c.SetKeepAlive(true)
		_ = c.SetKeepAlivePeriod(keepAlivePeriod)
	}
}

// acceptRetry backs off after failed accepts. When the process runs out of
// sockets every Accept fails at once; retrying in a tight loop pins a CPU,
// floods the log and makes recovery less likely.
type acceptRetry struct {
	delay time.Duration
}

// wait sleeps before the next attempt and reports whether the accept loop
// should continue.
func (a *acceptRetry) wait(ctx context.Context) bool {
	switch {
	case a.delay == 0:
		a.delay = 10 * time.Millisecond
	case a.delay < time.Second:
		a.delay *= 2
	default:
		a.delay = time.Second
	}

	timer := time.NewTimer(a.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (a *acceptRetry) reset() { a.delay = 0 }

// relayTraffic closes both connections on the first EOF, I/O error, or context
// cancellation, then joins both copy workers before returning. The caller owns
// shutdown; copy workers only report completion and never wait on one another.
func relayTraffic(ctx context.Context, src, dst net.Conn, logger *zap.Logger) {
	// Buffer both results so a worker can always report its exit, including
	// while the owner is closing sockets after cancellation.
	results := make(chan error, 2)
	pump := func(to, from net.Conn, direction string) {
		_, err := io.Copy(to, from)
		if err != nil {
			err = fmt.Errorf("%s: %w", direction, err)
		}
		results <- err
	}
	go pump(dst, src, "client to target")
	go pump(src, dst, "target to client")

	completed := 0
	var err error
	select {
	case err = <-results:
		completed++
	case <-ctx.Done():
	}
	// Closing both sockets unblocks reads and writes in the other direction.
	// This intentionally tears down the entire tunnel on a TCP half-close too.
	_ = src.Close()
	_ = dst.Close()
	for ; completed < 2; completed++ {
		if copyErr := <-results; err == nil {
			err = copyErr
		}
	}
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
			logger.Debug("connection closed (normal)", zap.Error(err))
		} else {
			logger.Warn("connection collapsed", zap.Error(err))
		}
	}
}

type rawAddr string

func (r rawAddr) Network() string { return "raw" }
func (r rawAddr) String() string  { return string(r) }

func addrFromRemote(remote string) net.Addr {
	if remote == "" {
		return rawAddr("")
	}
	if addr, err := net.ResolveTCPAddr("tcp", remote); err == nil {
		return addr
	}
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil && h != "" {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return &net.IPAddr{IP: ip}
	}
	return rawAddr(remote)
}

var ErrFieldMissing = errors.New("a mandatory field is missing")

func buildFieldMissing(service, field string) error {
	return fmt.Errorf("%w, router: %s, field: %s", ErrFieldMissing, service, field)
}
