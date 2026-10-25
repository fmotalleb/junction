package router

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/fmotalleb/junction/config"
	"github.com/fmotalleb/junction/proxy"
)

type handler func(context.Context, *Registry, int, config.EntryPoint) (bool, error) // Handled, error

type routeGroup struct {
	routing config.Router
	listen  netip.AddrPort
	entries []compiledEntry
}

type compiledEntry struct {
	entry        config.EntryPoint
	chain        *proxy.Chain
	transport    *http.Transport
	targetURL    *url.URL
	reqReplacer  *strings.Replacer
	respReplacer *strings.Replacer
}

// Registry holds the routers that can serve an entry point together with the
// cleanup they need once the server stops. Building one per server keeps the
// registration explicit instead of depending on package init() ordering.
type Registry struct {
	handlers []handler
	groups   map[string]routeGroup
	primary  map[int]bool
	routes   map[int]compiledEntry
}

// NewRegistry returns a registry with every built-in router registered.
// The order of the handlers does not matter: each router only claims entry
// points whose routing matches its own.
func NewRegistry() *Registry {
	r := &Registry{groups: make(map[string]routeGroup), primary: make(map[int]bool), routes: make(map[int]compiledEntry)}
	r.registerHandler(httpHandler)
	r.registerHandler(httpStaticHandler)
	r.registerHandler(httpToHTTPSHandler)
	r.registerHandler(sniRouter)
	r.registerHandler(tcpRouter)
	r.registerHandler(udpRouter)

	return r
}

func (r *Registry) registerHandler(h handler) {
	r.handlers = append(r.handlers, h)
}

// Prepare validates tagged groups and records their members before any listener
// starts. Entry order is retained, so overlapping rules always resolve the same way.
func (r *Registry) Prepare(entries []config.EntryPoint) error {
	r.groups = make(map[string]routeGroup)
	r.primary = make(map[int]bool)
	r.routes = make(map[int]compiledEntry)
	for index, entry := range entries {
		if err := validateEntryPoint(entry); err != nil {
			return fmt.Errorf("entry point %d: %w", index, err)
		}
		chain, err := proxy.NewChainWithDestinationPolicy(entry.Proxy, entry.DestinationAllow, entry.DestinationBlock)
		if err != nil {
			return fmt.Errorf("entry point %d has an invalid proxy chain: %w", index, err)
		}
		compiled := compiledEntry{entry: entry, chain: chain}
		if entry.Routing == config.RouterHTTPToHTTPS {
			compiled.targetURL, compiled.reqReplacer, compiled.respReplacer, err = buildHTTPSEntryConfig(entry)
			if err != nil {
				return fmt.Errorf("entry point %d has an invalid HTTPS route: %w", index, err)
			}
			if compiled.targetURL != nil && (compiled.targetURL.Scheme != "https" || compiled.targetURL.Host == "" || compiled.targetURL.User != nil) {
				return fmt.Errorf("entry point %d HTTPS target must have an HTTPS URL with a host and no userinfo", index)
			}
		}
		r.routes[index] = compiled
		if entry.Routing == config.RouterHTTPHeader || entry.Routing == config.RouterHTTPToHTTPS {
			compiled.transport = newProxyTransport(chain)
			r.routes[index] = compiled
		}
		if entry.Tag != nil && entry.Routing != config.RouterHTTPHeader && entry.Routing != config.RouterHTTPToHTTPS && entry.Routing != config.RouterSNI {
			return fmt.Errorf("entry point %d uses a tag with a router that does not support grouped routes", index)
		}
		if entry.Tag == nil {
			r.primary[index] = true
			continue
		}
		key := *entry.Tag
		if key == "" {
			return fmt.Errorf("entry point %d has an empty tag", index)
		}
		group, exists := r.groups[key]
		if exists && (group.routing != entry.Routing || group.listen != entry.Listen) {
			return fmt.Errorf("tag %q must use one routing mode and listen address", key)
		}
		if !exists {
			group = routeGroup{routing: entry.Routing, listen: entry.Listen}
			r.primary[index] = true
		}
		group.entries = append(group.entries, compiled)
		r.groups[key] = group
	}
	return nil
}

func validateEntryPoint(entry config.EntryPoint) error {
	routing := entry.Routing
	if !routing.IsValid() {
		return fmt.Errorf("invalid routing type %q", routing)
	}
	if !entry.Listen.IsValid() {
		return errors.New("listen address is required and must be host:port")
	}
	if (routing == config.RouterSNI || routing == config.RouterHTTPHeader) &&
		!entry.Listen.Addr().IsLoopback() && len(entry.AllowFrom) == 0 && !entry.PublicAccess {
		return errors.New("unrestricted non-loopback SNI/HTTP listeners require public_access = true or explicit allow_from rules")
	}
	if entry.Timeout < 0 {
		return errors.New("timeout cannot be negative")
	}
	if entry.GetTimeout() <= 0 {
		return errors.New("effective timeout must be positive; check TIMEOUT and the entry timeout")
	}
	if slices.Contains(entry.Features, flexiblePortFeature) && len(entry.AllowedPorts) == 0 {
		return errors.New("flexible-port requires an explicit allowed_ports list")
	}
	for _, port := range entry.AllowedPorts {
		if err := isValidPort(port); err != nil {
			return fmt.Errorf("invalid allowed_ports entry: %w", err)
		}
	}
	switch routing {
	case config.RouterSNI:
		if entry.Target != "" {
			if err := isValidPort(entry.Target); err != nil {
				return fmt.Errorf("SNI target must be a port: %w", err)
			}
		}
	case config.RouterHTTPHeader:
		if entry.Target != "" {
			if err := isValidPort(entry.Target); err != nil {
				return fmt.Errorf("HTTP target must be a port: %w", err)
			}
		}
	case config.RouterTCPRaw, config.RouterUDPRaw:
		host, port, err := net.SplitHostPort(entry.Target)
		if err != nil || host == "" {
			return errors.New("raw TCP/UDP target must be host:port")
		}
		if err := isValidPort(port); err != nil {
			return fmt.Errorf("raw TCP/UDP target: %w", err)
		}
	case config.RouterHTTPToHTTPS:
		if entry.Target == "" {
			return errors.New("http-to-https target URL is required for every route")
		}
	}
	return nil
}

func (r *Registry) Group(tag string) []config.EntryPoint {
	if r == nil {
		return nil
	}
	group := r.groups[tag]
	entries := make([]config.EntryPoint, 0, len(group.entries))
	for _, route := range group.entries {
		entries = append(entries, route.entry)
	}
	return entries
}

func (r *Registry) GroupRoutes(tag string) []compiledEntry {
	if r == nil {
		return nil
	}
	return append([]compiledEntry(nil), r.groups[tag].entries...)
}

func (r *Registry) Route(index int) (compiledEntry, bool) {
	if r == nil {
		return compiledEntry{}, false
	}
	route, ok := r.routes[index]
	return route, ok
}

// Handle runs the registered routers against e and stops at the first one that
// claims it. It returns an error when no router matches or a router rejects the
// configuration.
func (r *Registry) Handle(ctx context.Context, e config.EntryPoint) error {
	return r.handle(ctx, -1, e)
}

// HandleAt serves one entry from a prepared configuration. Tagged entries
// after the first are rules for the group's listener and do not bind again.
func (r *Registry) HandleAt(ctx context.Context, index int, e config.EntryPoint) error {
	if !r.primary[index] {
		return nil
	}
	return r.handle(ctx, index, e)
}

func (r *Registry) handle(ctx context.Context, index int, e config.EntryPoint) error {
	for _, h := range r.handlers {
		if handled, err := h(ctx, r, index, e); err != nil {
			return errors.Join(
				errors.New("handler denied the configuration"),
				err,
			)
		} else if handled {
			return nil
		}
	}
	return errors.New("no handler found for config")
}

// Reset runs the cleanup registered by the routers, dropping their group state.
func (r *Registry) Reset() {
	for _, route := range r.routes {
		if route.transport != nil {
			route.transport.CloseIdleConnections()
		}
	}
	r.groups = make(map[string]routeGroup)
	r.primary = make(map[int]bool)
	r.routes = make(map[int]compiledEntry)
}
