package router

import (
	"context"
	"errors"

	"github.com/fmotalleb/junction/config"
)

type handler func(context.Context, config.EntryPoint) error

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
		if err := h(ctx, e); err != nil {
			return errors.Join(
				errors.New("handler denied the configuration"),
				err,
			)
		}
	}
	return errors.New("no handler found for config")
}

func Reset() {
	for _, r := range reset {
		r()
	}
}
