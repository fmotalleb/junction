package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/fmotalleb/junction/config"
)

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

// relayTraffic concurrently relays data between two network connections in both directions until either connection is closed or an error occurs.
// It also enforces ctx: without the watcher below a tunnel whose peers went
// silent was held open until the process was restarted.
// Logs connection closure and errors for diagnostic purposes.
func relayTraffic(ctx context.Context, src, dst net.Conn, logger *zap.Logger) {
	defer func() {
		_ = src.Close()
		_ = dst.Close()
	}()

	errs, relayCtx := errgroup.WithContext(ctx)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-relayCtx.Done():
			_ = src.Close()
			_ = dst.Close()
		case <-stop:
		}
	}()

	errs.Go(
		func() error {
			if _, err := io.Copy(dst, src); err != nil {
				return errors.Join(errors.New("failed to write to dst"), err)
			}
			if halfCloser, ok := dst.(interface{ CloseWrite() error }); ok {
				return halfCloser.CloseWrite()
			}
			return nil
		},
	)
	errs.Go(
		func() error {
			if _, err := io.Copy(src, dst); err != nil {
				return errors.Join(errors.New("failed to write to dst"), err)
			}
			if halfCloser, ok := src.(interface{ CloseWrite() error }); ok {
				return halfCloser.CloseWrite()
			}
			return nil
		},
	)
	// Wait for the first error or closure
	err := errs.Wait()
	if err != nil {
		// A relay usually ends because one side closed, reset, or stopped
		// receiving. These are expected client/network conditions and can be
		// extremely frequent on a busy proxy, so retain details at debug level.
		logger.Debug("connection relay ended", zap.Error(err))
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
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.Addr)
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
