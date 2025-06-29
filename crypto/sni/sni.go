package sni

import (
	"crypto/tls"
	"encoding/binary"
	"errors"
)

const (
	tlsHandshakeTypeClientHello = 0x01
	tlsRecordTypeHandshake      = 0x16
	tlsRecordHeaderLen          = 5
	tlsHandshakeHeaderLen       = 4
)

// ClientHelloInfo holds extracted data from a TLS ClientHello.
type ClientHelloInfo struct {
	Version            uint16
	Random             [32]byte
	SessionID          []byte
	CipherSuites       []uint16
	CompressionMethods []byte
	SNIHostNames       []string
}

func (c *ClientHelloInfo) GetVersion() string {
	return tls.VersionName(c.Version)
}

func (c *ClientHelloInfo) GetCiphers() []string {
	ciphers := make([]string, len(c.CipherSuites))
	for i, c := range c.CipherSuites {
		ciphers[i] = tls.CipherSuiteName(c)
	}
	return ciphers
}

// ParseClientHello parses a TLS ClientHello and returns its information.
func ParseClientHello(data []byte) (*ClientHelloInfo, error) {
	if len(data) < tlsRecordHeaderLen+tlsHandshakeHeaderLen {
		return nil, errors.New("data too short for TLS record header")
	}
	if data[0] != tlsRecordTypeHandshake {
		return nil, errors.New("not a handshake record")
	}

	// TLS record length
	recordLength := int(binary.BigEndian.Uint16(data[3:5]))
	if len(data)-tlsRecordHeaderLen < recordLength {
		return nil, errors.New("incomplete record data")
	}

	handshake := data[tlsRecordHeaderLen:]
	if handshake[0] != tlsHandshakeTypeClientHello {
		return nil, errors.New("not a client hello")
	}
	handshakeLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if len(handshake)-4 < handshakeLen {
		return nil, errors.New("incomplete handshake data")
	}
	hello := handshake[4:]

	pos := 0
	if len(hello) < 2+32 {
		return nil, errors.New("client hello too short")
	}
	version := binary.BigEndian.Uint16(hello[pos:])
	pos += 2

	var random [32]byte
	copy(random[:], hello[pos:])
	pos += 32

	if pos >= len(hello) {
		return nil, errors.New("invalid session id length position")
	}
	sessionIDLen := int(hello[pos])
	pos++
	if pos+sessionIDLen > len(hello) {
		return nil, errors.New("invalid session id length")
	}
	sessionID := hello[pos : pos+sessionIDLen]
	pos += sessionIDLen

	if pos+2 > len(hello) {
		return nil, errors.New("cipher suites length out of bounds")
	}
	cipherLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if pos+cipherLen > len(hello) {
		return nil, errors.New("cipher suites out of bounds")
	}
	cipherCount := cipherLen / 2
	ciphers := make([]uint16, cipherCount)
	for i := 0; i < cipherCount; i++ {
		ciphers[i] = binary.BigEndian.Uint16(hello[pos+(i*2):])
	}
	pos += cipherLen

	if pos >= len(hello) {
		return nil, errors.New("compression methods length missing")
	}
	compMethodsLen := int(hello[pos])
	pos++
	if pos+compMethodsLen > len(hello) {
		return nil, errors.New("compression methods out of bounds")
	}
	compMethods := hello[pos : pos+compMethodsLen]
	pos += compMethodsLen

	if pos+2 > len(hello) {
		// No extensions present
		return &ClientHelloInfo{
			Version:            version,
			Random:             random,
			SessionID:          sessionID,
			CipherSuites:       ciphers,
			CompressionMethods: compMethods,
			SNIHostNames:       nil,
		}, nil
	}

	extLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if pos+extLen > len(hello) {
		return nil, errors.New("extensions out of bounds")
	}

	sniNames := []string{}
	extEnd := pos + extLen
	for pos+4 <= extEnd {
		extType := binary.BigEndian.Uint16(hello[pos:])
		extDataLen := int(binary.BigEndian.Uint16(hello[pos+2:]))
		pos += 4

		if pos+extDataLen > extEnd {
			break
		}

		if extType == 0x00 && extDataLen >= 2 {
			sniLen := int(binary.BigEndian.Uint16(hello[pos:]))
			if sniLen+2 <= extDataLen {
				sniEnd := pos + 2 + sniLen
				sniPos := pos + 2
				for sniPos+3 <= sniEnd {
					nameType := hello[sniPos]
					nameLen := int(binary.BigEndian.Uint16(hello[sniPos+1:]))
					sniPos += 3
					if sniPos+nameLen > sniEnd {
						break
					}
					if nameType == 0 {
						sniNames = append(sniNames, string(hello[sniPos:sniPos+nameLen]))
					}
					sniPos += nameLen
				}
			}
		}
		pos += extDataLen
	}

	return &ClientHelloInfo{
		Version:            version,
		Random:             random,
		SessionID:          sessionID,
		CipherSuites:       ciphers,
		CompressionMethods: compMethods,
		SNIHostNames:       sniNames,
	}, nil
}
