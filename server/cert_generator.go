package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"
)

// parsePrivateKey tries to parse a private key, supporting both PKCS#1 and PKCS#8 formats.
func parsePrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("failed to decode private key PEM")
	}

	switch block.Type {
	case "RSA PRIVATE KEY": // PKCS#1 format
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY": // PKCS#8 format
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
		}
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("unsupported private key type (not RSA)")
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", block.Type)
	}
}

// generateCert generates an SSL certificate, either self-signed or signed by a custom CA.
// Reads CA cert and key paths from environment variables.
func generateCert(cName string) ([]byte, []byte, error) {
	// Generate RSA private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   cName,
			Organization: []string{"InnerOrg"},
			Country:      []string{"US"},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},

		DNSNames: []string{cName},
	}

	caCertPath := os.Getenv("CA_CERT")
	caKeyPath := os.Getenv("CA_KEY")

	var certDER []byte
	if caCertPath != "" && caKeyPath != "" {
		caCertPEM, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caBlock, _ := pem.Decode(caCertPEM)
		if caBlock == nil || caBlock.Type != "CERTIFICATE" {
			return nil, nil, errors.New("invalid CA certificate")
		}
		caCert, err := x509.ParseCertificate(caBlock.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
		}

		caKeyPEM, err := os.ReadFile(caKeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read CA private key: %w", err)
		}
		caPrivKey, err := parsePrivateKey(caKeyPEM)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
		}

		certDER, err = x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caPrivKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
		}
	} else {
		certDER, err = x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
		}
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPEM, keyPEM, nil
}
