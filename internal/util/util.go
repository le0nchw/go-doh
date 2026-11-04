package util

import (
	"doh/internal/startup"
	"encoding/base64"
	"log"
)

// Decode base64 URL-encoded data
func DecodeBase64URL(data string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(data)
}

func EncodeBase64URL(data []byte) (string, error) {
	// ponytail: use RawURLEncoding to match DecodeBase64URL; it's URL-safe
	// (no +//=) so it round-trips cleanly in a ?dns= query param (F12).
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func GetDomainName(resp []byte) string {
	// ponytail: bounds-check every access so a crafted/short query can't read
	// out of range and panic (F7). Bail on compression pointers to avoid
	// misparsing.
	if len(resp) < 12 {
		return ""
	}

	// Skip the first 12 bytes (header)
	offset := 12

	// Read the domain name
	var domainName string
	for {
		if offset >= len(resp) {
			break
		}
		labelLen := int(resp[offset])
		if labelLen == 0 {
			break
		}
		if labelLen&0xC0 == 0xC0 { // compression pointer; stop parsing
			break
		}
		offset++
		if offset+labelLen > len(resp) {
			break
		}
		domainName += string(resp[offset : offset+labelLen])
		offset += labelLen
		if offset < len(resp) && resp[offset] != 0 {
			domainName += "."
		}
	}
	return domainName
}

func VLog(v ...any) {
	if *startup.Verbose {
		log.Println(v...)
	}
}
