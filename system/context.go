package system

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func NewSystemContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	// Set up a channel to listen for OS signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)

	// Goroutine to cancel the context when a signal is received
	go func() {
		<-signalChan
		cancel()
	}()

	return ctx
}
