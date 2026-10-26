package tls

import "encoding/binary"

const (
	tlsHandshakeFlag  = 0x16
	tlsHandshakeBegin = 0x01
	maxClientHelloLen = 64 << 10

	// HandshakeType(8 bit) + Length(24 bit) + Version(16 bit) + Random (256 bit).
	tlsHelloSize = 2 + 32
)

func ExtractSNI(data []byte) []byte {
	name, _ := ClientHelloStatus(data)
	return name
}

// ClientHelloStatus reports whether data contains a complete first ClientHello
// across one or more TLS records. A complete hello may legitimately omit SNI.
func ClientHelloStatus(data []byte) ([]byte, bool) {
	var handshake []byte
	for len(data) >= 5 {
		if data[0] != tlsHandshakeFlag {
			return nil, true
		}
		recordLen := int(binary.BigEndian.Uint16(data[3:5]))
		if recordLen == 0 || len(data)-5 < recordLen || len(handshake)+recordLen > maxClientHelloLen {
			return nil, true
		}
		handshake = append(handshake, data[5:5+recordLen]...)
		data = data[5+recordLen:]
		if len(handshake) < 4 {
			continue
		}
		if handshake[0] != tlsHandshakeBegin {
			return nil, true
		}
		messageLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
		if messageLen > maxClientHelloLen-4 {
			return nil, true
		}
		if len(handshake) >= 4+messageLen {
			return parseClientHelloSNI(handshake[4 : 4+messageLen]), true
		}
	}
	return nil, false
}

func parseClientHelloSNI(hello []byte) []byte {
	return findSNIExtension(clientHelloExtensions(hello))
}

func clientHelloExtensions(hello []byte) []byte {
	if len(hello) < tlsHelloSize {
		return nil
	}
	pos := tlsHelloSize
	if pos >= len(hello) {
		return nil
	}
	sessionIDLen := int(hello[pos])
	pos++
	if sessionIDLen > len(hello)-pos {
		return nil
	}
	pos += sessionIDLen
	if len(hello)-pos < 2 {
		return nil
	}
	cipherLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if cipherLen < 2 || cipherLen%2 != 0 || cipherLen > len(hello)-pos {
		return nil
	}
	pos += cipherLen
	if pos >= len(hello) {
		return nil
	}
	compressionLen := int(hello[pos])
	pos++
	if compressionLen > len(hello)-pos {
		return nil
	}
	pos += compressionLen
	if len(hello)-pos < 2 {
		return nil
	}
	extensionsLen := int(binary.BigEndian.Uint16(hello[pos:]))
	pos += 2
	if extensionsLen != len(hello)-pos {
		return nil
	}
	return hello[pos : pos+extensionsLen]
}

func findSNIExtension(extensions []byte) []byte {
	pos := 0
	end := len(extensions)
	for pos < end {
		if end-pos < 4 {
			return nil
		}
		extType := binary.BigEndian.Uint16(extensions[pos:])
		extDataLen := int(binary.BigEndian.Uint16(extensions[pos+2:]))
		pos += 4
		if extDataLen > end-pos {
			return nil
		}
		if extType == 0 {
			return parseSNIExtension(extensions[pos : pos+extDataLen])
		}
		pos += extDataLen
	}
	return nil
}

// parseSNIExtension returns the only real world case, a single name in sni packet.
func parseSNIExtension(sniData []byte) []byte {
	if len(sniData) < 5 || int(binary.BigEndian.Uint16(sniData[:2])) != len(sniData)-2 {
		return nil
	}
	listLen := int(binary.BigEndian.Uint16(sniData[:2]))
	if listLen < 3 || listLen > len(sniData)-2 {
		return nil
	}
	nameType := sniData[2]
	if nameType != 0 {
		return nil
	}
	nameLen := int(binary.BigEndian.Uint16(sniData[3:5]))
	if nameLen == 0 || nameLen != listLen-3 || nameLen > len(sniData)-5 {
		return nil
	}
	return sniData[5 : 5+nameLen]
}
