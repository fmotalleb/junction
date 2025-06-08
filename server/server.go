package server

import (
	"context"
	"errors"
	"sync"
	"syscall"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/go-tools/sysctx"
	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/router"
	"go.uber.org/zap"
)

func Serve(c config.Config) error {
	wg := new(sync.WaitGroup)

	ctx := context.Background()
	ctx = sysctx.CancelWith(ctx, syscall.SIGTERM)
	ctx, err := log.WithNewEnvLogger(ctx)
	if err != nil {
		return err
	}
	for _, e := range c.EntryPoints {
		wg.Add(1)
		go handleEntry(ctx, e, wg)
	}
	wg.Wait()
	return errors.New("every listener died")
}

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
