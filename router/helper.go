package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"
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
	dialCtx, cancel := proxy.WithDialTimeout(ctx)
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

const (
	maxConcurrentTunnels          = 1024
	maxConcurrentTunnelsPerSource = 64
)

const (
	maxHTTPConnectionsPerListener = 1024
	maxHTTPConnectionsPerSource   = 64
)

type sourceLimiter struct {
	mu     sync.Mutex
	active map[string]int
	limit  int
}

func newSourceLimiter(limit int) *sourceLimiter {
	return &sourceLimiter{active: make(map[string]int), limit: limit}
}

func (l *sourceLimiter) acquire(addr net.Addr) (func(), bool) {
	key := sourceIPKey(addr)
	l.mu.Lock()
	if l.active[key] >= l.limit {
		l.mu.Unlock()
		return nil, false
	}
	l.active[key]++
	l.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if l.active[key] <= 1 {
				delete(l.active, key)
			} else {
				l.active[key]--
			}
			l.mu.Unlock()
		})
	}, true
}

func sourceIPKey(addr net.Addr) string {
	if addr == nil {
		return "<unknown>"
	}
	host := addr.String()
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return ip.Unmap().String()
	}
	return host
}

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

func serveHTTPServer(ctx context.Context, server *http.Server) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	servingListener := listener
	if config.HardenedMode() {
		servingListener = newLimitedListener(listener, maxHTTPConnectionsPerListener)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(servingListener) }()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

type limitedListener struct {
	net.Listener
	permits chan struct{}
	sources *sourceLimiter
	done    chan struct{}
	close   sync.Once
}

func newLimitedListener(listener net.Listener, limit int) *limitedListener {
	return &limitedListener{
		Listener: listener,
		permits:  make(chan struct{}, limit),
		sources:  newSourceLimiter(maxHTTPConnectionsPerSource),
		done:     make(chan struct{}),
	}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		releaseSource, ok := l.sources.acquire(conn.RemoteAddr())
		if !ok {
			_ = conn.Close()
			continue
		}
		select {
		case l.permits <- struct{}{}:
			select {
			case <-l.done:
				<-l.permits
				releaseSource()
				_ = conn.Close()
				return nil, net.ErrClosed
			default:
				return &limitedConn{Conn: conn, release: func() {
					<-l.permits
					releaseSource()
				}}, nil
			}
		case <-l.done:
			releaseSource()
			_ = conn.Close()
			return nil, net.ErrClosed
		}
	}
}

func (l *limitedListener) Close() error {
	l.close.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitedConn struct {
	net.Conn
	release func()
	close   sync.Once
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.close.Do(c.release)
	return err
}

func (c *limitedConn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return nil
}

func (c *limitedConn) CloseRead() error {
	if conn, ok := c.Conn.(interface{ CloseRead() error }); ok {
		return conn.CloseRead()
	}
	return nil
}
