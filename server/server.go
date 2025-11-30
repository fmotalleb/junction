package server

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/sysctx"
	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/dns"
	"github.com/fmotalleb/junction/router"
	"github.com/fmotalleb/junction/services/singbox"
	"github.com/sethvargo/go-retry"
	"go.uber.org/zap"
)

// Serve starts server components based on the provided configuration, including optional Singbox integration, and waits for all entry points to complete.
// Serve starts all configured server entry points and the optional Singbox service, blocking until all listeners have stopped.
// Returns an error if logger initialization fails or if all listeners exit.
func Serve(c config.Config) error {
	wg := new(sync.WaitGroup)

	ctx := context.Background()
	ctx = sysctx.CancelWith(ctx, syscall.SIGTERM)
	ctx, err := log.WithNewEnvLogger(ctx)
	dns.Serve(ctx, c.Core.FakeDNS)
	if err != nil {
		return err
	}
	if len(c.Core.SingboxCfg) != 0 {
		go runSingbox(ctx, c.Core.SingboxCfg)
	}
	if c.Core.FakeDNS != nil {
		go dns.Serve(ctx, c.Core.FakeDNS)
	}
	for _, e := range c.EntryPoints {
		wg.Add(1)
		go handleEntry(ctx, e, wg)
	}
	wg.Wait()
	return errors.New("every listener died")
}

// runSingbox starts the Singbox service with the provided configuration and context.
// Returns an error if Singbox fails to start.
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

// handleEntry starts handling the specified entry point within the given context and marks the wait group as done when finished.
// Logs a warning if the entry point handler fails to start.
func handleEntry(ctx context.Context, e config.EntryPoint, wg *sync.WaitGroup) {
	defer wg.Done()
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
