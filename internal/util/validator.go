package util

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
)

// DNSRequestValidator is a struct that represents a DNS request validator
type DNSRequestValidator struct {
	Data []byte
}

var dnsRequestValidatorPool = sync.Pool{
	New: func() interface{} {
		return &DNSRequestValidator{}
	},
}

// NewDNSRequestValidator creates a new instance of DNSRequestValidator with the given data
func NewDNSRequestValidator(data []byte) *DNSRequestValidator {
	v := dnsRequestValidatorPool.Get().(*DNSRequestValidator)
	v.Data = data
	return v
}

// Validate checks if the DNS request binary is valid
func (v *DNSRequestValidator) Validate() error {
	defer dnsRequestValidatorPool.Put(v)
	// Check if the data is at least 12 bytes long
	if len(v.Data) < 12 {
		return fmt.Errorf("DNS request binary is too short")
	}

	// Check the header
	var header DNSHeader
	err := binary.Read(bytes.NewReader(v.Data[:12]), binary.BigEndian, &header)
	if err != nil {
		return fmt.Errorf("failed to read DNS header: %v", err)
	}

	// Check the header fields
	if header.QuestionCount == 0 {
		return fmt.Errorf("no questions in DNS request")
	}

	// Parse the questions
	offset := 12
	for i := 0; i < int(header.QuestionCount); i++ {
		var question DNSQuestion
		err := parseDNSName(v.Data, &offset, &question.Name)
		if err != nil {
			return fmt.Errorf("failed to parse DNS question name: %v", err)
		}

		// Ensure the slice has enough capacity before reading the type and class
		if offset+4 > len(v.Data) {
			return fmt.Errorf("DNS request binary is too short")
		}
		err = binary.Read(bytes.NewReader(v.Data[offset:offset+4]), binary.BigEndian, &question.Type)
		if err != nil {
			return fmt.Errorf("failed to read DNS question type: %v", err)
		}
		err = binary.Read(bytes.NewReader(v.Data[offset+2:offset+4]), binary.BigEndian, &question.Class)
		if err != nil {
			return fmt.Errorf("failed to read DNS question class: %v", err)
		}
		offset += 4
	}
	return nil
}

// DNSHeader represents the header of a DNS request
type DNSHeader struct {
	ID              uint16
	Flags           uint16
	QuestionCount   uint16
	AnswerCount     uint16
	AuthorityCount  uint16
	AdditionalCount uint16
}

// DNSQuestion represents a DNS question
type DNSQuestion struct {
	Name  string
	Type  uint16
	Class uint16
}

// parseDNSName parses a DNS name from the binary data
func parseDNSName(data []byte, offset *int, name *string) error {
	var nameBytes []byte
	for {
		if *offset >= len(data) {
			return fmt.Errorf("DNS request binary is too short")
		}
		labelLen := int(data[*offset])
		if labelLen == 0 {
			*offset += 1
			break
		}
		*offset += 1
		if *offset+labelLen > len(data) {
			return fmt.Errorf("DNS request binary is too short")
		}
		nameBytes = append(nameBytes, data[*offset:*offset+labelLen]...)
		nameBytes = append(nameBytes, '.')
		*offset += labelLen
	}
	if len(nameBytes) > 0 {
		nameBytes = nameBytes[:len(nameBytes)-1] // Remove the trailing dot
	}
	*name = string(nameBytes)
	return nil
}
