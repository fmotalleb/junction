package server

import (
	"net/http"

	"github.com/FMotalleb/junction/config"
)

func serveHttp(target config.Target) error {
	handler, err := newProxyHandler(target.Proxy, &target.Target)
	if err != nil {
		return err
	}
	listenAddr := target.GetListenAddr()
	return http.ListenAndServe(listenAddr, handler)
}
