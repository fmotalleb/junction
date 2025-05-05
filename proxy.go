package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const socksProxyAddr = "127.0.0.1:7890" // change as needed

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: ./proxyserver [http:port] [https:port]")
	}

	for _, arg := range os.Args[1:] {
		parts := strings.Split(arg, ":")
		if len(parts) != 2 {
			log.Fatalf("Invalid argument: %s", arg)
		}

		proto := strings.ToLower(parts[0])
		port := parts[1]

		switch proto {
		case "http":
			go startHTTP(":" + port)
		case "https":
			go startHTTPS(":" + port)
		default:
			log.Fatalf("Unknown protocol: %s", proto)
		}
	}

	select {} // block forever
}

func startHTTP(addr string) {
	log.Printf("Starting HTTP server on %s\n", addr)
	http.ListenAndServe(addr, newProxyHandler())
}

func startHTTPS(addr string) {
	log.Printf("Starting HTTPS server on %s\n", addr)
	cert, key, err := generateSelfSignedCert()
	if err != nil {
		log.Fatalf("Failed to generate cert: %v", err)
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		log.Fatalf("TLS load error: %v", err)
	}

	server := &http.Server{
		Addr:      addr,
		Handler:   newProxyHandler(),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{tlsCert}},
	}

	log.Fatal(server.ListenAndServeTLS("", ""))
}

func newProxyHandler() http.Handler {
	dialer, err := proxy.SOCKS5("tcp", socksProxyAddr, nil, proxy.Direct)
	if err != nil {
		log.Fatalf("SOCKS5 dialer error: %v", err)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Disable certificate validation
		},

		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}

	return &httputil.ReverseProxy{
		Director: func(req *http.Request) {

			target, _ := url.Parse("https://192.168.250.42/") // change as needed
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host

		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			// optionally modify response
			return nil
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Proxy error: %v", err)
			http.Error(w, "Proxy error", http.StatusBadGateway)
		},
	}
}

func generateSelfSignedCert() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))

	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certBuf, keyBuf, nil
}
