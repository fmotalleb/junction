package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/dns"
	"github.com/fmotalleb/junction/router"
	"github.com/fmotalleb/junction/services/singbox"
	"github.com/sethvargo/go-retry"
	"go.uber.org/zap"
)

// Serve starts server components based on the provided configuration, including optional Singbox integration, and waits for all entry points to complete.
// Serve starts all configured server entry points and the optional Singbox service, blocking until all listeners have stopped.
// It returns an error if logger initialization fails or when all listeners have exited.
func Serve(ctx context.Context, c config.Config) error {
	wg := new(sync.WaitGroup)
	defer router.Reset()
	if len(c.Core.SingboxCfg) != 0 {
		wg.Go(
			func() {
				runSingbox(ctx, c.Core.SingboxCfg)
			},
		)
	}
	if c.Core.FakeDNS != nil {
		wg.Go(
			func() {
				runDNS(ctx, c.Core.FakeDNS)
			},
		)
	}
	for _, e := range c.EntryPoints {
		wg.Go(
			func() {
				handleEntry(ctx, e)
			},
		)
	}
	wg.Wait()
	select {
	case <-ctx.Done(): // normal behavior is context cancellation
		return nil
	default: // If wait group is done without context cancellation its an error in configuration
		return errors.New("every listener died")
	}
}

// runSingbox starts the Singbox service with the provided configuration and context.
// runSingbox starts and supervises the Singbox service using the provided context and configuration.
// It retries on failures with an exponential backoff, logs each crash, and panics if the service has an unrecoverable crash.
func runSingbox(ctx context.Context, cfg map[string]any) {
	l := log.Of(ctx)
	b := BuildBackoff()
	err := retry.Do(ctx, b, func(ctx context.Context) error {
		err := singbox.Start(ctx, cfg)
		if err != nil {
			l.Error("singbox crashed", zap.Error(err))
		}
		return err
	})
	if err != nil {
		l.Panic("singbox had unrecoverable crash", zap.Error(err))
	}
}

// runDNS starts the DNS service with the provided configuration and context.
// runDNS starts the FakeDNS service using cfg, retrying with an exponential backoff on failure.
// It logs each crash and panics if the service cannot be started after retries are exhausted.
func runDNS(ctx context.Context, cfg *config.FakeDNS) {
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
		l.Panic("dns had unrecoverable crash", zap.Error(err))
	}
}

// handleEntry starts handling the specified entry point within the given context and marks the wait group as done when finished.
// handleEntry starts handling the given entry point and marks the wait group as done when it returns.
// If starting the handler fails, it logs a warning that includes the entry and the error.
func handleEntry(ctx context.Context, e config.EntryPoint) {
	if err := router.Handle(ctx, e); err != nil {
		log.
			FromContext(ctx).
			Named("handleEntry").
			Warn(
				"failed to start handler",
				zap.Any("entry", e),
				zap.Error(err),
			)
		return
	}
}

func BuildBackoff() retry.Backoff {
	backoff := retry.NewExponential(time.Second)
	backoff = retry.WithCappedDuration(time.Second*16, backoff)
	return backoff
}
