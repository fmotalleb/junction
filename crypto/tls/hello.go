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

func (out *ClientHello) parseClientHelloBody(hello []byte) (int, error) {
	pos := 0
	if len(hello) < 2+32 {
		return 0, errors.New("client hello too short")
	}
	out.Version = binary.BigEndian.Uint16(hello[pos:])
	pos += 2

	copy(out.Random[:], hello[pos:pos+32])
	pos += 32

	sessionIDLen, err := getSessionIDLen(hello, pos)
	if err != nil {
		return 0, err
	}
	pos++
	out.SessionID = hello[pos : pos+sessionIDLen]
	pos += sessionIDLen

	cipherLen, err := getCipherLen(hello, pos)
	if err != nil {
		return 0, err
	}
	pos += 2
	cipherCount := cipherLen / 2
	out.CipherSuites = out.CipherSuites[:0]
	for i := 0; i < cipherCount; i++ {
		cs := binary.BigEndian.Uint16(hello[pos+(i*2):])
		out.CipherSuites = append(out.CipherSuites, cs)
	}
	pos += cipherLen

	compMethodsLen, err := getCompMethodsLen(hello, pos)
	if err != nil {
		return 0, err
	}
	pos++
	out.CompressionMethods = hello[pos : pos+compMethodsLen]
	pos += compMethodsLen
	return pos, nil
}

func (out *ClientHello) Unmarshal(buf []byte) error {
	err := checkHeaders(buf)
	if err != nil {
		return err
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
	pos, err := out.parseClientHelloBody(hello)
	if err != nil {
		return err
	}
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
	out.parseExtensions(hello, pos, extEnd)
	return nil
}

func checkHeaders(buf []byte) error {
	if err := checkRecordHeader(buf); err != nil {
		return err
	}
	if err := checkHandshakeHeader(buf); err != nil {
		return err
	}
	return nil
}

// parseSNIExtension parses the SNI extension data and populates SNIHostNames and SNICount.
func (out *ClientHello) parseSNIExtension(data []byte) {
	if len(data) < 2 {
		return
	}
	sniLen := int(binary.BigEndian.Uint16(data))
	sniEnd := 2 + sniLen
	sniPos := 2
	for sniPos+3 <= sniEnd && out.SNICount < len(out.SNIHostNames) && sniEnd <= len(data) {
		nameType := data[sniPos]
		nameLen := int(binary.BigEndian.Uint16(data[sniPos+1:]))
		sniPos += 3
		if sniPos+nameLen > sniEnd {
			break
		}
		if nameType == 0 {
			out.SNIHostNames[out.SNICount] = data[sniPos : sniPos+nameLen]
			out.SNICount++
		}
		sniPos += nameLen
	}
}

// parseExtensions parses the extensions in the ClientHello message.
func (out *ClientHello) parseExtensions(hello []byte, pos, extEnd int) {
	for pos+4 <= extEnd {
		extType := binary.BigEndian.Uint16(hello[pos:])
		extDataLen := int(binary.BigEndian.Uint16(hello[pos+2:]))
		pos += 4

		if pos+extDataLen > extEnd {
			break
		}

		if extType == 0x00 && extDataLen >= 2 {
			out.parseSNIExtension(hello[pos : pos+extDataLen])
		}
		pos += extDataLen
	}
}

// Helper functions to reduce cyclomatic complexity

func checkRecordHeader(buf []byte) error {
	if len(buf) < tlsRecordHeaderLen+tlsHandshakeHeaderLen {
		return errors.New("data too short")
	}
	if buf[0] != tlsRecordTypeHandshake {
		return errors.New("not a handshake")
	}
	return nil
}

func checkHandshakeHeader(buf []byte) error {
	if len(buf) < tlsRecordHeaderLen+tlsHandshakeHeaderLen {
		return errors.New("data too short for handshake")
	}
	return nil
}

func getSessionIDLen(hello []byte, pos int) (int, error) {
	if pos >= len(hello) {
		return 0, errors.New("invalid session id")
	}
	sessionIDLen := int(hello[pos])
	if pos+1+sessionIDLen > len(hello) {
		return 0, errors.New("invalid session id length")
	}
	return sessionIDLen, nil
}

func getCipherLen(hello []byte, pos int) (int, error) {
	if pos+2 > len(hello) {
		return 0, errors.New("invalid cipher suites")
	}
	cipherLen := int(binary.BigEndian.Uint16(hello[pos:]))
	if pos+2+cipherLen > len(hello) {
		return 0, errors.New("cipher suites too long")
	}
	return cipherLen, nil
}

func getCompMethodsLen(hello []byte, pos int) (int, error) {
	if pos >= len(hello) {
		return 0, errors.New("no compression methods")
	}
	compMethodsLen := int(hello[pos])
	if pos+1+compMethodsLen > len(hello) {
		return 0, errors.New("compression methods out of bounds")
	}
	return compMethodsLen, nil
}
