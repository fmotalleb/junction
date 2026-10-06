package front

import (
	"context"
	"embed"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"go.uber.org/zap"
)

//go:generate npm ci
//go:generate npm run build

//go:embed all:dist
var distFS embed.FS

const shutdownGrace = 5 * time.Second

// getDist returns a filesystem rooted at the embedded "dist" directory.
// It enables access to static files embedded at compile time.
func getDist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Serve starts an HTTP server on the specified address, serving embedded static files from the "dist" directory at the root path.
// The server logs connection state changes and applies one-minute timeouts for read, write, and idle operations.
// Returns an error if initialization fails or if the server encounters an error while running.
func Serve(ctx context.Context, listen string) error {
	dist, err := getDist()
	if err != nil {
		return err
	}
	logger := log.Of(ctx)

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(dist)))

	logger.Sugar().Infof("Server started on http://%s", listen)

	server := &http.Server{
		Addr:    listen,
		Handler: mux,
		ConnState: func(nc net.Conn, s http.ConnState) {
			logger.Debug("connection state update",
				zap.String("state", s.String()),
				zap.String("client", nc.RemoteAddr().String()),
			)
		},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		WriteTimeout:      time.Minute,
		IdleTimeout:       time.Minute,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
