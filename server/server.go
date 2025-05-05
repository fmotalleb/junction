package server

import (
	"fmt"
	"log"

	"github.com/FMotalleb/junction/config"
)

func Serve(cfg config.Config) error {
	for _, target := range cfg.Targets {
		go func() {
			if err := serve(target); err != nil {
				log.Fatal("server failed", err)
			}
		}()
	}
	return nil
}

func serve(target config.Target) error {
	switch target.Target.Scheme {
	case "http":
		return serveHttp(target)
	case "https":
		return serveHttps(target)
	default:
		return fmt.Errorf("schema: %s is not supported", target.Target.Scheme)
	}
}
