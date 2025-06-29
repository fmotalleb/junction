package server

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"time"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/go-tools/sysctx"
	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/router"
	"github.com/FMotalleb/junction/services/singbox"
	"go.uber.org/zap"
)

// Serve starts server components based on the provided configuration, including optional Singbox integration, and waits for all entry points to complete.
// Returns an error if initialization fails or if all listeners have stopped.
func Serve(c config.Config) error {
	wg := new(sync.WaitGroup)

	ctx := context.Background()
	ctx = sysctx.CancelWith(ctx, syscall.SIGTERM)
	logBuilders := make([]log.BuilderFunc, 0)
	ctx, err := log.WithNewEnvLogger(ctx, logBuilders...)
	if err != nil {
		return err
	}
	if len(c.Core.SingboxCfg) != 0 {
		go runSingbox(ctx, c.Core.SingboxCfg)
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
	maxTry := 50
	tryCount := 0
	for tryCount < maxTry {
		err := singbox.Start(ctx, cfg)
		if err != nil {
			tryCount++
			go func() {
				<-time.After(time.Second)
				tryCount = 0
			}()
			l.Error("singbox error", zap.Error(err))
		}
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
