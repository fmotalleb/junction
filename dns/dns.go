package dns

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/matcher"
	"github.com/fmotalleb/junction/config"
	"github.com/miekg/dns"
	"github.com/sethvargo/go-retry"
	"go.uber.org/zap"
)

type handler struct {
	answer    net.IP // IP to return
	forwarder string // upstream DNS (e.g., "8.8.8.8:53")
	allowList []matcher.Matcher
	ctx       context.Context
}

// Serve starts and runs a fake DNS server based on the provided configuration.
// The server will answer A queries with cfg.ReturnAddr for names allowed by cfg.Allowed,
// optionally forward other queries to cfg.Forwarder, and listen on cfg.Listen (default 0.0.0.0:53).
// It returns an error if cfg.ReturnAddr is not provided or if the UDP listener cannot be created.
// If the server exits with an error while the provided context is still active, that error is wrapped
// as retry.RetryableError; if the context is done, Serve returns nil.
func Serve(ctx context.Context, cfg config.FakeDNS) error {
	logger := log.Of(ctx).Named("DNS")
	sCtx := log.WithLogger(ctx, logger)
	if cfg.ReturnAddr == nil {
		return errors.New("fake DNS requires a return address (answer) to be configured")
	}
	forwarder := ""
	if cfg.Forwarder != nil {
		forwarder = cfg.Forwarder.String()
	}
	h := &handler{
		ctx:       sCtx,
		answer:    *cfg.ReturnAddr, // e.g., "10.0.0.1"
		forwarder: forwarder,       // e.g., "1.1.1.1:53"
		allowList: cfg.Allowed,
	}
	listenAddr := "0.0.0.0:53"
	if cfg.Listen != nil {
		listenAddr = cfg.Listen.String()
	}
	l, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		logger.Error("failed to start server", zap.Error(err))
		return err
	}
	go func() {
		<-ctx.Done()
		logger.Info("context deadline reached")
		if err := l.Close(); err != nil {
			logger.Info("failed to close listener", zap.Error(err))
		}
	}()

	if serverErr := dns.ActivateAndServe(nil, l, h); serverErr != nil {
		select {
		case <-ctx.Done():
			return nil
		default:
			return retry.RetryableError(serverErr)
		}
	}
	return nil
}

func (h *handler) IsAllowed(question string) bool {
	if len(h.allowList) == 0 {
		return true
	}
	q := strings.TrimRight(question, ".")
	for _, m := range h.allowList {
		if m.Match(q) {
			return true
		}
	}
	return false
}

func (h *handler) logger() *zap.Logger {
	return log.Of(h.ctx)
}

// ServeDNS implements dns.Handler.
func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)

	if len(r.Question) == 0 {
		if err := w.WriteMsg(msg); err != nil {
			h.logger().Info("failed to write answer to empty question", zap.Error(err))
		}
		return
	}

	q := r.Question[0]
	logger := h.logger().WithLazy(
		zap.String("name", q.Name),
		zap.Uint16("class", q.Qclass),
		zap.Uint16("type", q.Qtype),
	)
	// Always respond with your answer for A requests
	if q.Qtype == dns.TypeA && h.IsAllowed(q.Name) {
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    10,
			},
			A: h.answer,
		}
		msg.Answer = append(msg.Answer, rr)
		if err := w.WriteMsg(msg); err != nil {
			logger.Warn("failed to write answer", zap.Error(err))
		}
		return
	}

	if h.forwarder == "" {
		msg = msg.SetRcode(r, dns.RcodeRefused)
		if err := w.WriteMsg(msg); err != nil {
			logger.Warn("failed to write refused answer", zap.Error(err))
		}
		return
	}
	ctx, cancel := context.WithTimeout(h.ctx, time.Second*10)
	defer cancel()

	// Otherwise forward to upstream
	resp, exchangeErr := dns.ExchangeContext(ctx, r, h.forwarder)
	if exchangeErr != nil {
		logger.Warn("failed to read result from forwarder", zap.Error(exchangeErr))
		// If forward failed, still return NXDOMAIN instead of crashing
		fallback := new(dns.Msg)
		fallback.SetRcode(r, dns.RcodeNameError)
		if err := w.WriteMsg(fallback); err != nil {
			logger.Warn("failed to write fallback answer", zap.Error(err))
		}
		return
	}

	if err := w.WriteMsg(resp); err != nil {
		logger.Warn("failed to write answer from forwarder", zap.Error(err))
	}
}