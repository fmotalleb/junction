package server

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/FMotalleb/junction/config"
)

func serveHttps(target config.Target) error {

	cert, key, err := generateCert(target.Target.Host)
	if err != nil {
		return fmt.Errorf("Failed to generate cert: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return fmt.Errorf("TLS load error: %w", err)
	}
	handler, err := newProxyHandler(target.Proxy, &target.Target)
	if err != nil {
		return err
	}
	listenAddr := target.GetListenAddr()
	tlsConf := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	server := &http.Server{
		Addr:      listenAddr,
		Handler:   handler,
		TLSConfig: tlsConf,
	}

	return server.ListenAndServeTLS("", "")
}
