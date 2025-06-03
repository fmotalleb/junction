package server

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/router"
	"github.com/FMotalleb/junction/system"
)

func Serve(c config.Config) error {
	wg := new(sync.WaitGroup)
	ctx := system.NewSystemContext()
	for _, t := range c.Targets {
		wg.Add(1)
		go handleBgServer(ctx, t, wg)
	}
	wg.Wait()
	return errors.New("every listener died")
}

func handleBgServer(ctx context.Context, t config.Target, wg *sync.WaitGroup) {
	defer wg.Done()
	if err := router.Handle(ctx, t); err != nil {
		log.Fatalf("failed to start handlers: %e", err)
		return
	}
}
