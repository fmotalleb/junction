package router

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/fmotalleb/junction/proxy"
	"github.com/fmotalleb/junction/utils"
)

func dialTarget(proxyAddr []*url.URL, target string, logger *zap.Logger) (net.Conn, error) {
	dialer, err := proxy.NewDialer(proxyAddr)
	if err != nil {
		logger.Error("failed to create SOCKS5 dialer", zap.Error(err))
		return nil, err
	}

	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		logger.Debug("failed to connect to target", zap.Error(err))
		return nil, err
	}
	return conn, nil
}

// relayTraffic concurrently relays data between two network connections in both directions until either connection is closed or an error occurs.
// Logs connection closure and errors for diagnostic purposes.
func relayTraffic(ctx context.Context, src, dst net.Conn, logger *zap.Logger) {
	defer func() {
		_ = src.Close()
		_ = dst.Close()
	}()
	errs, _ := errgroup.WithContext(ctx)
	errs.Go(
		func() error {
			if err := utils.Copy(dst, src); err != nil {
				return errors.Join(errors.New("failed to write to dst"), err)
			} else {
				return nil
			}
		},
	)
	errs.Go(
		func() error {
			if err := utils.Copy(src, dst); err != nil {
				return errors.Join(errors.New("failed to write to dst"), err)
			} else {
				return nil
			}
		},
	)
	// Wait for the first error or closure
	err := errs.Wait()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			logger.Debug("connection closed (normal)", zap.Error(err))
		} else {
			logger.Warn("connection collapsed", zap.Error(err))
		}
	}
}

type rawAddr string

func (r rawAddr) Network() string { return "raw" }
func (r rawAddr) String() string  { return string(r) }

func addrFromRemote(remote string) net.Addr {
	if remote == "" {
		return rawAddr("")
	}
	if addr, err := net.ResolveTCPAddr("tcp", remote); err == nil {
		return addr
	}
	host := remote
	if h, _, err := net.SplitHostPort(remote); err == nil && h != "" {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return &net.IPAddr{IP: ip}
	}
	return rawAddr(remote)
}

var ErrFieldMissing = errors.New("a mandatory field is missing")

func buildFieldMissing(service, field string) error {
	return fmt.Errorf("%w, router: %s, field: %s", ErrFieldMissing, service, field)
}
