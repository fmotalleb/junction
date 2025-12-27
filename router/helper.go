package router

import (
	"errors"
	"net"
	"net/url"
	"sync"

	"github.com/fmotalleb/junction/proxy"
	"github.com/fmotalleb/junction/utils"
	"go.uber.org/zap"
)

func dialTarget(proxyAddr []*url.URL, target string, logger *zap.Logger) (net.Conn, error) {
	dialer, err := proxy.NewDialer(proxyAddr)
	if err != nil {
		logger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return nil, err
	}

	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		logger.Error("failed to connect to target", zap.Error(err))
		return nil, err
	}
	return conn, nil
}

// relayTraffic concurrently relays data between two network connections in both directions until either connection is closed or an error occurs.
// Logs connection closure and errors for diagnostic purposes.
func relayTraffic(src, dst net.Conn, logger *zap.Logger) {
	var once sync.Once
	errCh := make(chan error, 2)

	closeBoth := func() {
		_ = src.Close()
		_ = dst.Close()
	}

	// Copy from src to dst
	go func() {
		err := utils.Copy(dst, src)
		if err != nil {
			errCh <- errors.Join(errors.New("failed to write to dst"), err)
		} else {
			errCh <- nil
		}
	}()

	// Copy from dst to src
	go func() {
		err := utils.Copy(src, dst)
		if err != nil {
			errCh <- errors.Join(errors.New("failed to write to src"), err)
		} else {
			errCh <- nil
		}
	}()

	// Wait for the first error or closure
	err := <-errCh
	once.Do(closeBoth)

	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			logger.Debug("connection closed (normal)", zap.Error(err))
		} else {
			logger.Warn("connection collapsed", zap.Error(err))
		}
	}
}
