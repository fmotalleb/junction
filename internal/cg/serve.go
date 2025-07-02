package cg

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/go-tools/sysctx"
	"github.com/FMotalleb/junction/internal/cg/front"
	"go.uber.org/zap"
)

func Serve(listen string) error {
	ctx := context.Background()
	ctx = sysctx.CancelWith(ctx, syscall.SIGTERM)
	ctx, err := log.WithNewEnvLogger(ctx)
	if err != nil {
		return err
	}
	dist, err := front.GetDist()
	if err != nil {
		return err
	}
	log := log.Of(ctx)
	http.Handle("/", http.FileServer(http.FS(dist)))

	log.Info("Server started", zap.String("listen", listen))

	server := &http.Server{
		Addr: listen,
		ConnState: func(nc net.Conn, s http.ConnState) {
			log.Info("connection state update",
				zap.String("state", s.String()),
				zap.String("client", nc.RemoteAddr().String()),
			)
		},
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
		IdleTimeout:  time.Minute,
	}
	return server.ListenAndServe()
}
