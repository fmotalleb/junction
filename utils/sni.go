package utils

func ExtractSNI(data []byte) string {
	if len(data) < 43 {
		return ""
	}

	// Check if it's a TLS handshake
	if data[0] != 0x16 {
		return ""
	}

	// Skip to extensions
	pos := 43
	if pos >= len(data) {
		return ""
	}

	// Skip session ID
	sessionIDLen := int(data[pos])
	pos += 1 + sessionIDLen
	if pos+2 >= len(data) {
		return ""
	}

	// Skip cipher suites
	cipherSuitesLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2 + cipherSuitesLen
	if pos+1 >= len(data) {
		return ""
	}

	// Skip compression methods
	compressionMethodsLen := int(data[pos])
	pos += 1 + compressionMethodsLen
	if pos+2 >= len(data) {
		return ""
	}

	// Extensions length
	extensionsLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2

	end := pos + extensionsLen
	for pos < end && pos+4 < len(data) {
		extType := int(data[pos])<<8 | int(data[pos+1])
		extLen := int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4

		if extType == 0 { // SNI extension
			if pos+5 < len(data) {
				// Skip list length and name type
				nameLen := int(data[pos+3])<<8 | int(data[pos+4])
				if pos+5+nameLen <= len(data) {
					return string(data[pos+5 : pos+5+nameLen])
				}
			}
		}
		pos += extLen
	}

	return ""
}
