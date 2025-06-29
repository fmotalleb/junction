package tls

import (
	"encoding/binary"
	"errors"
)

const (
	tlsHandshakeTypeClientHello = 0x01
	tlsRecordTypeHandshake      = 0x16
	tlsRecordHeaderLen          = 5
	tlsHandshakeHeaderLen       = 4
)

type ClientHello struct {
	Version            uint16
	Random             [32]byte
	SessionID          []byte
	CipherSuites       []uint16
	CompressionMethods []byte
	SNIHostNames       [4][]byte // limit to 4 names to avoid dynamic append
	SNICount           int
}

func (out *ClientHello) Unmarshal(buf []byte) error {
	if len(buf) < tlsRecordHeaderLen+tlsHandshakeHeaderLen {
		return errors.New("data too short")
	}
	if buf[0] != tlsRecordTypeHandshake {
		return errors.New("not a handshake")
	}

	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if len(buf)-tlsRecordHeaderLen < recordLen {
		return errors.New("truncated record")
	}
	handshake := buf[tlsRecordHeaderLen:]
	if handshake[0] != tlsHandshakeTypeClientHello {
		return errors.New("not a client hello")
	}

	hello := handshake[tlsHandshakeHeaderLen:]
	pos := 0
	if len(hello) < 2+32 {
		return errors.New("client hello too short")
	}
	out.Version = binary.BigEndian.Uint16(hello[pos:])
	pos += 2

	copy(out.Random[:], hello[pos:pos+32])
	pos += 32

	if pos >= len(hello) {
		return errors.New("invalid session id")
	}
	sessionIDLen := int(hello[pos])
	pos++
	if pos+sessionIDLen > len(hello) {
		return errors.New("invalid session id length")
	}
	out.SessionID = hello[pos : pos+sessionIDLen]
	pos += sessionIDLen

	if pos+2 > len(hello) {
		return errors.New("invalid cipher suites")
	}
	cipherLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if pos+cipherLen > len(hello) {
		return errors.New("cipher suites too long")
	}
	cipherCount := cipherLen / 2
	out.CipherSuites = out.CipherSuites[:0]
	for i := 0; i < cipherCount; i++ {
		cs := binary.BigEndian.Uint16(hello[pos+(i*2):])
		out.CipherSuites = append(out.CipherSuites, cs)
	}
	pos += cipherLen

	if pos >= len(hello) {
		return errors.New("no compression methods")
	}
	compMethodsLen := int(hello[pos])
	pos++
	if pos+compMethodsLen > len(hello) {
		return errors.New("compression methods out of bounds")
	}
	out.CompressionMethods = hello[pos : pos+compMethodsLen]
	pos += compMethodsLen

	if pos+2 > len(hello) {
		// No extensions
		out.SNICount = 0
		return nil
	}

	extLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if pos+extLen > len(hello) {
		return errors.New("extensions truncated")
	}

	extEnd := pos + extLen
	out.SNICount = 0
	for pos+4 <= extEnd {
		extType := binary.BigEndian.Uint16(hello[pos:])
		extDataLen := int(binary.BigEndian.Uint16(hello[pos+2:]))
		pos += 4

		if pos+extDataLen > extEnd {
			break
		}

		if extType == 0x00 && extDataLen >= 2 {
			sniLen := int(binary.BigEndian.Uint16(hello[pos:]))
			sniEnd := pos + 2 + sniLen
			sniPos := pos + 2
			for sniPos+3 <= sniEnd && out.SNICount < len(out.SNIHostNames) {
				nameType := hello[sniPos]
				nameLen := int(binary.BigEndian.Uint16(hello[sniPos+1:]))
				sniPos += 3
				if sniPos+nameLen > sniEnd {
					break
				}
				if nameType == 0 {
					out.SNIHostNames[out.SNICount] = hello[sniPos : sniPos+nameLen]
					out.SNICount++
				}
				sniPos += nameLen
			}
		}
		pos += extDataLen
	}
	return nil
}
