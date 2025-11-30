package dns

import (
	"context"
	"net"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/junction/config"
	"github.com/miekg/dns"
	"go.uber.org/zap"
)

type handler struct {
	logger    *zap.Logger
	answer    string // IP to return
	forwarder string // upstream DNS (e.g., "8.8.8.8:53")
}

func Serve(ctx context.Context, cfg *config.FakeDNS) error {
	logger := log.Of(ctx).Named("DNS")
	ans := ""
	if cfg.ReturnAddr != nil {
		ans = cfg.ReturnAddr.String()
	}
	forwarder := ""
	if cfg.Forwarder != nil {
		forwarder = cfg.Forwarder.String()
	}
	h := &handler{
		logger:    logger,
		answer:    ans,       // e.g., "10.0.0.1"
		forwarder: forwarder, // e.g., "1.1.1.1:53"
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

	return dns.ActivateAndServe(nil, l, h)
}

// ServeDNS implements dns.Handler.
func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)

	if len(r.Question) == 0 {
		if err := w.WriteMsg(msg); err != nil {
			h.logger.Info("failed to write answer to empty question", zap.Error(err))
		}
		return
	}

	q := r.Question[0]
	logger := h.logger.WithLazy(zap.String("question", q.String()))
	// Always respond with your answer for A requests
	if q.Qtype == dns.TypeA {
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    10,
			},
			A: net.ParseIP(h.answer),
		}
		msg.Answer = append(msg.Answer, rr)
		if err := w.WriteMsg(msg); err != nil {
			logger.Warn("failed to write answer", zap.Error(err))
		}
		return
	}

	if h.forwarder == "" {
		if err := w.WriteMsg(r); err != nil {
			logger.Warn("failed to write nil answer", zap.Error(err))
		}
		return
	}
	// Otherwise forward to upstream
	resp, exchangeErr := dns.Exchange(r, h.forwarder)
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
