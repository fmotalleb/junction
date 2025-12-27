package router

import (
	"context"
	"errors"

	"github.com/fmotalleb/junction/config"
)

type handler func(context.Context, config.EntryPoint) (bool, error) // Handled, error

var (
	handlers = []handler{}
	reset    = []func(){}
)

func registerHandler(h handler) {
	handlers = append(handlers, h)
}

func registerReset(r func()) {
	reset = append(reset, r)
}

func Handle(ctx context.Context, e config.EntryPoint) error {
	for _, h := range handlers {
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

func Reset() {
	for _, r := range reset {
		r()
	}
}
