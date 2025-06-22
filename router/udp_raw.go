package router

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/FMotalleb/go-tools/log"
	"github.com/FMotalleb/junction/config"
	"go.uber.org/zap"
)

func init() {
	registerHandler(udpRouter)
}

func udpRouter(ctx context.Context, entry config.EntryPoint) error {
	if entry.Routing != "udp-raw" {
		return nil
	}

	logger := log.FromContext(ctx).
		Named("router.udp-raw").
		With(zap.Any("entry", entry))

	addrPort := entry.GetListenAddr()
	udpAddr := net.UDPAddrFromAddrPort(addrPort)
	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		logger.Error("failed to listen", zap.String("addr", addrPort.String()), zap.Error(err))
		return err
	}
	defer listener.Close()

	if entry.Target == "" {
		logger.Error("UDP proxy must have a target ip:port address")
		return errors.New("router: udp-raw must have `to` field")
	}

	logger.Info("raw UDP proxy booted")

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		buff := make([]byte, 1)
		_, addr, err := listener.ReadFrom(buff)
		if err != nil {
			if ctx.Err() != nil {
				logger.Info("listener closed due to context cancellation")
				return nil
			}
			logger.Error("failed to accept connection", zap.Error(err))
			continue
		}
		c := &udpConn{
			addr:   addr,
			buffer: buff,
		}
		go handleUDPConnection(ctx, logger, c, entry)
	}
}

type udpConn struct {
	addr   net.Addr
	buffer []byte
}

// Close implements net.Conn.
func (u *udpConn) Close() error {
	panic("unimplemented")
}

// LocalAddr implements net.Conn.
func (u *udpConn) LocalAddr() net.Addr {
	panic("unimplemented")
}

// Read implements net.Conn.
func (u *udpConn) Read(b []byte) (n int, err error) {
	panic("unimplemented")
}

// RemoteAddr implements net.Conn.
func (u *udpConn) RemoteAddr() net.Addr {
	panic("unimplemented")
}

// SetDeadline implements net.Conn.
func (u *udpConn) SetDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetReadDeadline implements net.Conn.
func (u *udpConn) SetReadDeadline(t time.Time) error {
	panic("unimplemented")
}

// SetWriteDeadline implements net.Conn.
func (u *udpConn) SetWriteDeadline(t time.Time) error {
	panic("unimplemented")
}

// Write implements net.Conn.
func (u *udpConn) Write(b []byte) (n int, err error) {
	panic("unimplemented")
}

func handleUDPConnection(parentCtx context.Context, logger *zap.Logger, conn net.Conn, entry config.EntryPoint) {
	ctx, cancel := context.WithTimeout(parentCtx, entry.GetTimeout())
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	targetConn, err := DialTarget(entry.Proxy, entry.Target, logger)
	if err != nil {
		return
	}
	defer targetConn.Close()

	RelayTraffic(conn, targetConn, logger)
}
