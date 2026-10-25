package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/sethvargo/go-retry"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/dns"
	"github.com/fmotalleb/junction/router"
)

var ErrServerDiedBeforeContext = errors.New("context is not canceled but listeners are dead")

// Serve starts server components based on the provided configuration, including optional Singbox integration, and waits for all entry points to complete.
// Serve starts all configured server entry points and the optional Singbox service, blocking until all listeners have stopped.
// It returns an error if logger initialization fails or when all listeners have exited.
func Serve(ctx context.Context, c config.Config) error {
	if len(c.EntryPoints) == 0 && c.Core.FakeDNS == nil {
		if ctx.Err() != nil {
			return nil
		}
		return ErrServerDiedBeforeContext
	}
	reg := router.NewRegistry()
	defer reg.Reset()
	if err := reg.Prepare(c.EntryPoints); err != nil {
		return fmt.Errorf("validate route groups: %w", err)
	}
	group, serviceCtx := errgroup.WithContext(ctx)
	if c.Core.FakeDNS != nil {
		group.Go(func() error { return runDNS(serviceCtx, c.Core.FakeDNS) })
	}
	if c.Core.Pprof.Enable {
		pprofListen := c.Core.Pprof.Listen
		if pprofListen == "" {
			pprofListen = "127.0.0.1:6060"
		}
		group.Go(func() error { return servePprof(serviceCtx, pprofListen) })
	}
	for index, e := range c.EntryPoints {
		entryIndex := index
		entry := e
		group.Go(func() error { return handleEntry(serviceCtx, reg, entryIndex, entry) })
	}
	if err := group.Wait(); err != nil {
		return err
	}
	select {
	case <-ctx.Done(): // normal behavior is context cancellation
		return nil
	default: // If wait group is done without context cancellation its an error in configuration
		return ErrServerDiedBeforeContext
	}
}

func servePprof(ctx context.Context, listen string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	server := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serveDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-serveDone:
		}
	}()
	err := server.ListenAndServe()
	close(serveDone)
	<-watcherDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// runDNS starts the DNS service with the provided configuration and context.
// runDNS starts the FakeDNS service using cfg, retrying with an exponential backoff on failure.
// It logs every crash and returns the final error once the backoff is exhausted, so the
// caller can stop the process with a message instead of a panic stack trace.
func runDNS(ctx context.Context, cfg *config.FakeDNS) error {
	l := log.Of(ctx)
	b := BuildBackoff()
	err := retry.Do(ctx, b, func(ctx context.Context) error {
		err := dns.Serve(ctx, *cfg)
		if err != nil {
			l.Error("dns crashed", zap.Error(err))
		}
		return err
	})
	if err != nil {
		l.Error("dns had unrecoverable crash", zap.Error(err))
		return errors.Join(errors.New("fake dns stopped for good"), err)
	}
	return nil
}

// handleEntry starts handling the specified entry point within the given context and marks the wait group as done when finished.
// handleEntry starts handling the given entry point and marks the wait group as done when it returns.
// If starting the handler fails, it logs a warning that includes the entry and the error.
func handleEntry(ctx context.Context, reg *router.Registry, index int, e config.EntryPoint) error {
	warnOpenListener(ctx, e)
	if err := reg.HandleAt(ctx, index, e); err != nil {
		log.
			FromContext(ctx).
			Named("handleEntry").
			Warn(
				"failed to start handler",
				zap.String("router", string(e.Routing)),
				zap.String("listen", e.Listen.String()),
				zap.Error(err),
			)
		return err
	}
	return nil
}

func BuildBackoff() retry.Backoff {
	backoff := retry.NewExponential(time.Second)
	backoff = retry.WithCappedDuration(time.Second*16, backoff)
	return backoff
}

// warnOpenListener logs a loud warning when an entry point binds a non-loopback
// address without allow_from, because that combination makes the process an open
// proxy for everybody who can reach the address.
func warnOpenListener(ctx context.Context, e config.EntryPoint) {
	if len(e.AllowFrom) != 0 || !e.Listen.IsValid() || e.Listen.Addr().IsLoopback() {
		return
	}
	log.
		FromContext(ctx).
		Named("handleEntry").
		Warn(
			"entry point is reachable from outside the machine but has no allow_from; anyone who can connect may use it as an open proxy",
			zap.String("listen", e.Listen.String()),
		)
}
