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
	allowFrom []matcher.Matcher
	inFlight  chan struct{}
	ctx       context.Context
}

const maxDNSConcurrentRequests = 256

type resolver struct {
	ranger cidranger.Ranger
	answer net.IP
}

func newResolvers(returnAddr []*config.DNSResult) ([]resolver, error) {
	resolvers := make([]resolver, len(returnAddr))
	for index, e := range returnAddr {
		if e == nil {
			return nil, fmt.Errorf("fake DNS answer entry %d is nil", index)
		}
		if e.Result == nil {
			return nil, fmt.Errorf("fake DNS answer entry %d has no result IP configured", index)
		}
		if e.Result.To4() == nil {
			return nil, fmt.Errorf("fake DNS answer entry %d must contain an IPv4 address", index)
		}
		ranger := cidranger.NewPCTrieRanger()
		for _, r := range e.From {
			if r == nil {
				return nil, fmt.Errorf("fake DNS answer entry %d has a nil source range", index)
			}
			err := ranger.Insert(
				cidranger.NewBasicRangerEntry(*r),
			)
			if err != nil {
				return nil, err
			}
		}
		resolvers[index] = resolver{
			ranger: ranger,
			answer: *e.Result,
		}
	}
	return resolvers, nil
}

func newHandler(ctx context.Context, resolvers []resolver, forwarder string, allowList []matcher.Matcher) *handler {
	return &handler{
		ctx:       ctx,
		resolvers: resolvers,
		forwarder: forwarder,
		allowList: allowList,
		inFlight:  make(chan struct{}, maxDNSConcurrentRequests),
	}
}

func listen(ctx context.Context, listenAddr string) (net.PacketConn, error) {
	listener := new(net.ListenConfig)
	l, err := listener.ListenPacket(ctx, "udp", listenAddr)
	if err != nil {
		return nil, err
	}
	return l, nil
}

// Serve starts and runs a fake DNS server based on the provided configuration.
// The server will answer A queries with cfg.ReturnAddr for names allowed by cfg.Allowed,
// optionally forward other queries to cfg.Forwarder, and listen on cfg.Listen (default 0.0.0.0:53).
// It returns an error if cfg.ReturnAddr is not provided or if the UDP listener cannot be created.
// If the server exits with an error while the provided context is still active, that error is wrapped
// Serve starts and runs a fake DNS server configured by cfg.
// It validates configuration (requires at least one ReturnAddr entry with a result IP),
// builds per-entry CIDR resolvers that map client source ranges to response IPs, and
// binds a UDP listener on cfg.Listen or "127.0.0.1:53" to handle DNS queries.
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

	if err := validateDNSConfig(cfg); err != nil {
		return err
	}

	forwarder := ""
	if cfg.Forwarder != nil {
		forwarder = cfg.Forwarder.String()
	}

	resolvers, err := newResolvers(cfg.ReturnAddr)
	if err != nil {
		return err
	}

	h := newHandler(sCtx, resolvers, forwarder, cfg.Allowed)
	h.allowFrom = cfg.AllowFrom
	listenAddr := ":53"
	if config.HardenedMode() {
		listenAddr = "127.0.0.1:53"
	}
	if cfg.Listen != nil {
		listenAddr = cfg.Listen.String()
	}
	l, err := listen(ctx, listenAddr)
	if err != nil {
		logger.Error("failed to start server", zap.Error(err))
		return err
	}
	return serveListener(ctx, logger, l, h)
}

func validateDNSConfig(cfg config.FakeDNS) error {
	if len(cfg.ReturnAddr) == 0 {
		return errors.New("fake DNS requires a return address (answer) to be configured")
	}
	if config.HardenedMode() && cfg.Listen != nil && !cfg.Listen.Addr().IsLoopback() && len(cfg.AllowFrom) == 0 {
		return errors.New("fake DNS bound outside loopback requires allow_from client rules")
	}
	return nil
}

func serveListener(ctx context.Context, logger *zap.Logger, l net.PacketConn, h *handler) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			if err := l.Close(); err != nil {
				logger.Debug("failed to close listener", zap.Error(err))
			}
		case <-stop:
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

func (h *handler) IsAllowedClient(addr net.Addr) bool {
	if len(h.allowFrom) == 0 {
		return true
	}
	if addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	for _, rule := range h.allowFrom {
		if rule.Match(host) {
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

func (h *handler) handleARecord(w dns.ResponseWriter, r *dns.Msg, q dns.Question) {
	answer := h.findAnswer(w.RemoteAddr())
	if answer == nil {
		h.sendRefusedResponse(w, r, h.logger())
		return
	}
	msg := new(dns.Msg)
	msg.SetReply(r)
	rr := &dns.A{
		Hdr: dns.RR_Header{
			Name:   q.Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    10,
		},
		A: answer.To4(),
	}
	msg.Answer = append(msg.Answer, rr)
	if err := w.WriteMsg(msg); err != nil {
		h.logger().Warn("failed to write answer", zap.Error(err))
	}
}

func (h *handler) forwardRequest(w dns.ResponseWriter, r *dns.Msg, logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(h.ctx, time.Second*10)
	defer cancel()

	resp, exchangeErr := exchangeUpstream(ctx, r, h.forwarder)
	if exchangeErr != nil {
		logger.Debug("failed to read result from forwarder", zap.Error(exchangeErr))
		// Upstream failures are temporary server failures, not authoritative
		// statements that the requested name does not exist.
		fallback := new(dns.Msg)
		fallback.SetRcode(r, dns.RcodeServerFailure)
		if err := w.WriteMsg(fallback); err != nil {
			logger.Warn("failed to write fallback answer", zap.Error(err))
		}
		return
	}

	if err := w.WriteMsg(resp); err != nil {
		logger.Warn("failed to write answer from forwarder", zap.Error(err))
	}
}

func exchangeUpstream(ctx context.Context, request *dns.Msg, forwarder string) (*dns.Msg, error) {
	client := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	response, _, err := client.ExchangeContext(ctx, request, forwarder)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("upstream DNS returned an empty response")
	}
	if !response.Truncated {
		return response, nil
	}
	response, _, err = (&dns.Client{Net: "tcp", Timeout: 5 * time.Second}).ExchangeContext(ctx, request, forwarder)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("upstream DNS returned an empty TCP response")
	}
	return response, nil
}

func (h *handler) sendRefusedResponse(w dns.ResponseWriter, r *dns.Msg, logger *zap.Logger) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.SetRcode(r, dns.RcodeRefused)
	if err := w.WriteMsg(msg); err != nil {
		logger.Warn("failed to write refused answer", zap.Error(err))
	}
}

func (h *handler) handleEmptyQuestion(w dns.ResponseWriter, r *dns.Msg) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	if err := w.WriteMsg(msg); err != nil {
		h.logger().Info("failed to write answer to empty question", zap.Error(err))
	}
}

// ServeDNS implements dns.Handler.
func (h *handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if !h.IsAllowedClient(w.RemoteAddr()) {
		h.sendRefusedResponse(w, r, h.logger())
		return
	}
	if config.HardenedMode() {
		select {
		case h.inFlight <- struct{}{}:
			defer func() { <-h.inFlight }()
		default:
			h.sendRefusedResponse(w, r, h.logger())
			return
		}
	}
	if len(r.Question) == 0 {
		h.handleEmptyQuestion(w, r)
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

	if q.Qtype == dns.TypeA && h.IsAllowed(q.Name) {
		h.handleARecord(w, r, q)
		return
	}

	if h.forwarder == "" {
		h.sendRefusedResponse(w, r, logger)
		return
	}
	h.forwardRequest(w, r, logger)
}
