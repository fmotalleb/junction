package server

import (
	"context"
	"errors"
	"sync"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/router"
	"github.com/FMotalleb/junction/system"
	"github.com/FMotalleb/log"
	"go.uber.org/zap"
)

func Serve(c config.Config) error {
	wg := new(sync.WaitGroup)
	ctx := system.NewSystemContext()
	ctx, err := log.WithNewEnvLogger(ctx)
	if err != nil {
		return err
	}
	for _, t := range c.Targets {
		wg.Add(1)
		go handleTarget(ctx, t, wg)
	}
	wg.Wait()
	return errors.New("every listener died")
}

func handleTarget(ctx context.Context, t config.Target, wg *sync.WaitGroup) {
	defer wg.Done()
	l := log.FromContext(ctx).Named("handleTarget")
	if err := router.Handle(ctx, t); err != nil {
		l.Warn("failed to start handler", zap.Any("target", t), zap.Error(err))
		return
	}
}
