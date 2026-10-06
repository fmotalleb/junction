package router

import (
	"context"
	"errors"

	"github.com/fmotalleb/junction/config"
)

type handler func(context.Context, config.EntryPoint) (bool, error) // Handled, error

// Registry holds the routers that can serve an entry point together with the
// cleanup they need once the server stops. Building one per server keeps the
// registration explicit instead of depending on package init() ordering.
type Registry struct {
	handlers []handler
	reset    []func()
}

// NewRegistry returns a registry with every built-in router registered.
// The order of the handlers does not matter: each router only claims entry
// points whose routing matches its own.
func NewRegistry() *Registry {
	r := &Registry{}
	r.registerHandler(httpHandler)
	r.registerHandler(httpToHTTPSHandler)
	r.registerHandler(sniRouter)
	r.registerHandler(tcpRouter)
	r.registerHandler(udpRouter)

	r.registerReset(func() {
		httpGroupMu.Lock()
		httpGroups = make(map[string][]config.EntryPoint)
		httpGroupMu.Unlock()
	})
	r.registerReset(func() {
		httpsGroupMu.Lock()
		httpsGroups = make(map[string][]config.EntryPoint)
		httpsGroupMu.Unlock()
	})
	r.registerReset(func() {
		groupMu.Lock()
		sniGroups = make(map[string][]config.EntryPoint)
		groupMu.Unlock()
	})
	return r
}

func (r *Registry) registerHandler(h handler) {
	r.handlers = append(r.handlers, h)
}

func (r *Registry) registerReset(fn func()) {
	r.reset = append(r.reset, fn)
}

// Handle runs the registered routers against e and stops at the first one that
// claims it. It returns an error when no router matches or a router rejects the
// configuration.
func (r *Registry) Handle(ctx context.Context, e config.EntryPoint) error {
	for _, h := range r.handlers {
		if handled, err := h(ctx, e); err != nil {
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
	for _, fn := range r.reset {
		fn()
	}
}
