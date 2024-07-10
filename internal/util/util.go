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
	return base64.StdEncoding.EncodeToString(data), nil
}

func GetDomainName(resp []byte) string {
	// Skip the first 12 bytes (header)
	offset := 12

	// Read the domain name
	var domainName string
	for {
		labelLen := int(resp[offset])
		if labelLen == 0 {
			break
		}
		offset++
		domainName += string(resp[offset : offset+labelLen])
		offset += labelLen
		if resp[offset] != 0 {
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
