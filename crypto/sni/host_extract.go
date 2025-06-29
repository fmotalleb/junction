package sni

import "encoding/binary"

const (
	tlsHandshakeFlag  = 0x16
	tlsHandshakeBegin = 0x16
	tlsHelloSize      = 4 + 2 + 32
)

func ExtractHost(data []byte) []byte {
	if !isTLSHandshake(data) {
		return nil
	}
	handshake := data[5:]
	pos, ok := skipClientHelloHeaders(handshake)
	if !ok {
		return nil
	}
	sni := extractSNIFromExtensions(handshake, pos)
	return sni
}

func isTLSHandshake(data []byte) bool {
	if len(data) < 9 || data[0] != tlsHandshakeFlag {
		return false
	}
	handshake := data[5:]
	return handshake[0] != tlsHandshakeBegin
}

func skipClientHelloHeaders(handshake []byte) (int, bool) {
	pos := tlsHelloSize
	if pos > len(handshake) {
		return 0, false
	}
	sidLen := int(handshake[pos])
	pos += 1 + sidLen
	if pos+2 > len(handshake) {
		return 0, false
	}
	csLen := int(binary.BigEndian.Uint16(handshake[pos:]))
	pos += 2 + csLen
	if pos+1 > len(handshake) {
		return 0, false
	}
	compMethodsLen := int(handshake[pos])
	pos += 1 + compMethodsLen
	if pos+2 > len(handshake) {
		return 0, false
	}
	return pos, true
}

func extractSNIFromExtensions(handshake []byte, pos int) []byte {
	if pos+2 > len(handshake) {
		return nil
	}
	extLen := int(binary.BigEndian.Uint16(handshake[pos:]))
	pos += 2
	if pos > len(handshake) {
		return nil
	}
	end := pos + extLen
	if end > len(handshake) {
		return nil
	}
	for pos+4 <= end {
		extType := binary.BigEndian.Uint16(handshake[pos:])
		extDataLen := binary.BigEndian.Uint16(handshake[pos+2:])
		pos += 4
		if pos+int(extDataLen) > len(handshake) {
			break
		}
		if extType == 0x00 {
			sniData := handshake[pos : pos+int(extDataLen)]
			return parseSNIExtension(sniData)
		}
		pos += int(extDataLen)
	}
	return nil
}

// parseSNIExtension returns the only real world case a single name in sni packet.
func parseSNIExtension(sniData []byte) []byte {
	if len(sniData) < 5 {
		return nil
	}
	nameType := sniData[2]
	if nameType != 0 {
		return nil
	}
	nameLen := int(binary.BigEndian.Uint16(sniData[3:5])) + 5
	if nameLen > len(sniData) {
		return nil
	}
	return sniData[5:nameLen]
}
