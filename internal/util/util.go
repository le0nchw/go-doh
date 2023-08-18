package util

import (
	"encoding/base64"
)

// Decode base64 URL-encoded data
func DecodeBase64URL(data string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(data)
}
