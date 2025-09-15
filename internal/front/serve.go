package front

import (
	"context"
	"embed"
	"io/fs"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/sysctx"
	"go.uber.org/zap"
)

//go:generate npm i
//go:generate npm run build

//go:embed dist/*
var distFS embed.FS

// getDist returns a filesystem rooted at the embedded "dist" directory.
// It enables access to static files embedded at compile time.
func getDist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Serve starts an HTTP server on the specified address, serving embedded static files from the "dist" directory at the root path.
// The server logs connection state changes and applies one-minute timeouts for read, write, and idle operations.
// Returns an error if initialization fails or if the server encounters an error while running.
func Serve(listen string) error {
	ctx := context.Background()
	ctx = sysctx.CancelWith(ctx, syscall.SIGTERM)
	ctx, err := log.WithNewEnvLogger(ctx)
	if err != nil {
		return err
	}
	dist, err := getDist()
	if err != nil {
		return err
	}
	log := log.Of(ctx)
	http.Handle("/", http.FileServer(http.FS(dist)))

	log.Sugar().Infof("Server started on http://%s", listen)

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
