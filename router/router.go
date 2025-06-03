package router

import (
	"context"
	"errors"

	"github.com/FMotalleb/junction/config"
)

type handler func(context.Context, config.Target) error

var handlers = []handler{}

func registerHandler(h handler) {
	handlers = append(handlers, h)
}

func Handle(ctx context.Context, t config.Target) error {
	for _, h := range handlers {
		if err := h(ctx, t); err != nil {
			return errors.Join(
				errors.New("handler denied the configuration"),
				err,
			)
		}
	}
	return errors.New("no handler found for config")
}
