package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/fmotalleb/go-tools/log"
	"github.com/fmotalleb/go-tools/matcher"
	"github.com/miekg/dns"
	"github.com/sethvargo/go-retry"
	"go.uber.org/zap"

	"github.com/fmotalleb/junction/config"

	"github.com/yl2chen/cidranger"
)

type handler struct {
	resolvers []resolver // IP to return
	forwarder string     // upstream DNS (e.g., "8.8.8.8:53")
	allowList []matcher.Matcher
	ctx       context.Context
}

type resolver struct {
	ranger cidranger.Ranger
	answer net.IP
}

// Serve starts and runs a fake DNS server based on the provided configuration.
// The server will answer A queries with cfg.ReturnAddr for names allowed by cfg.Allowed,
// optionally forward other queries to cfg.Forwarder, and listen on cfg.Listen (default 0.0.0.0:53).
// It returns an error if cfg.ReturnAddr is not provided or if the UDP listener cannot be created.
// If the server exits with an error while the provided context is still active, that error is wrapped
// Serve starts and runs a fake DNS server configured by cfg.
// It validates configuration (requires at least one ReturnAddr entry with a result IP),
// builds per-entry CIDR resolvers that map client source ranges to response IPs, and
// binds a UDP listener on cfg.Listen or "0.0.0.0:53" to handle DNS queries.
// A-type queries for names allowed by cfg.Allowed are answered from the matching resolver;
// other queries are forwarded to cfg.Forwarder when configured or refused otherwise.
// The listener is closed when ctx is done.
//
// The function returns nil on successful shutdown or when the provided context is done.
// It returns an error for configuration or binding failures (e.g., missing result IP or listen error).
// Runtime server errors are returned wrapped as retry.RetryableError unless the context is done.
func Serve(ctx context.Context, cfg config.FakeDNS) error {
	logger := log.Of(ctx).Named("DNS")
	sCtx := log.WithLogger(ctx, logger)

	if len(cfg.ReturnAddr) == 0 {
		return errors.New("fake DNS requires a return address (answer) to be configured")
	}

	forwarder := ""
	if cfg.Forwarder != nil {
		forwarder = cfg.Forwarder.String()
	}

	resolvers := make([]resolver, len(cfg.ReturnAddr))
	for index, e := range cfg.ReturnAddr {
		if e.Result == nil {
			return fmt.Errorf("fake DNS answer entry %d has no result IP configured", index)
		}
		ranger := cidranger.NewPCTrieRanger()
		for _, r := range e.From {
			err := ranger.Insert(
				cidranger.NewBasicRangerEntry(*r),
			)
			if err != nil {
				return err
			}
		}
		resolvers[index] = resolver{
			ranger: ranger,
			answer: *e.Result,
		}
	}

	h := &handler{
		ctx:       sCtx,
		resolvers: resolvers, // e.g., "10.0.0.1"
		forwarder: forwarder, // e.g., "1.1.1.1:53"
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
	logger.Info("dns server started")
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

func (h *handler) findAnswer(a net.Addr) net.IP {
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		h.logger().Error("failed to parse remote address", zap.String("addr", a.String()), zap.Error(err))
		return nil
	}
	src := net.ParseIP(host)
	for _, r := range h.resolvers {
		if ok, err := r.ranger.Contains(src); ok {
			return r.answer
		} else if err != nil {
			h.logger().Error("error happened trying to match ip with ranger", zap.Error(err))
		}
	}
	return nil
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
		zap.String("from", w.RemoteAddr().String()),
	)
	logger.Debug("handling dns request")
	// Always respond with your answer for A requests
	if q.Qtype == dns.TypeA && h.IsAllowed(q.Name) {
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    10,
			},
			A: h.findAnswer(w.RemoteAddr()),
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
