package utils

import "encoding/binary"

const (
	tlsHandshakeFlag  = 0x16
	tlsHandshakeBegin = 0x16
	tlsHelloSize      = 4 + 2 + 32
)

func ExtractSNI(data []byte) string {
	if !isTLSHandshake(data) {
		return ""
	}
	handshake := data[5:]
	pos, ok := skipClientHelloHeaders(handshake)
	if !ok {
		return ""
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

func extractSNIFromExtensions(handshake []byte, pos int) string {
	if pos+2 > len(handshake) {
		return ""
	}
	extLen := int(binary.BigEndian.Uint16(handshake[pos:]))
	pos += 2
	if pos > len(handshake) {
		return ""
	}
	end := pos + extLen
	if end > len(handshake) {
		return ""
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
	return ""
}

func parseSNIExtension(sniData []byte) string {
	if len(sniData) < 5 {
		return ""
	}
	nameType := sniData[2]
	if nameType != 0 {
		return ""
	}
	nameLen := int(binary.BigEndian.Uint16(sniData[3:5]))
	if 5+nameLen > len(sniData) {
		return ""
	}
	return string(sniData[5 : 5+nameLen])
}
