package util

import (
	"encoding/base64"
	"strings"
)

// Decode base64 URL-encoded data
func DecodeBase64URL(data string) ([]byte, error) {
	// Convert base64 URL encoded string to standard base64 encoding
	data = strings.ReplaceAll(data, "-", "+")
	data = strings.ReplaceAll(data, "_", "/")

	// Add padding if necessary
	pad := len(data) % 4
	if pad != 0 {
		data += strings.Repeat("=", 4-pad)
	}

	// Decode base64 data
	return base64.StdEncoding.DecodeString(data)
}
