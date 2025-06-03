package router

import (
	"context"
	"log"
	"net"
	"strconv"

	"github.com/FMotalleb/junction/config"
	"github.com/FMotalleb/junction/utils"
	"golang.org/x/net/proxy"
)

func init() {
	registerHandler(sniRouter)
}

func sniRouter(ctx context.Context, target config.Target) error {
	if target.Routing != "sni" {
		return nil
	}
	listener, err := net.Listen("tcp", target.GetListenAddr())
	targetPort := "443"
	if target.TargetPort != 0 {
		targetPort = strconv.Itoa(target.TargetPort)
	}
	log.Printf("SNI proxy listening on port %d", target.ListenPort)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		go handleSNIConnection(conn, target.Proxy, targetPort)
	}
}

func handleSNIConnection(clientConn net.Conn, proxyAddr, targetPort string) {
	defer clientConn.Close()

	// Read the first few bytes to extract SNI
	buffer := make([]byte, 4096)
	n, err := clientConn.Read(buffer)
	if err != nil {
		log.Printf("Failed to read from client: %v", err)
		return
	}

	serverName := utils.ExtractSNI(buffer[:n])
	if serverName == "" {
		log.Printf("Failed to extract SNI from connection")
		return
	}

	log.Printf("Extracted SNI: %s", serverName)

	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	if err != nil {
		log.Printf("Failed to create SOCKS5 dialer: %v", err)
		return
	}

	// Connect to target server through SOCKS5 proxy
	targetConn, err := dialer.Dial("tcp", net.JoinHostPort(serverName, targetPort))
	if err != nil {
		log.Printf("Failed to connect to target %s: %v", serverName, err)
		return
	}
	defer targetConn.Close()

	// Send the initial buffer to target
	if _, err := targetConn.Write(buffer[:n]); err != nil {
		log.Printf("Failed to write initial buffer to target: %v", err)
		return
	}

	// Start bidirectional copying
	go utils.Copy(clientConn, targetConn)
	utils.Copy(targetConn, clientConn)
}
