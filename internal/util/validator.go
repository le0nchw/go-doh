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

// Validate checks if the DNS request binary is a well-formed query.
//
// It performs a full, bounds-checked parse of the message header, question
// section, and the answer / authority / additional sections so that malformed
// input cannot be forwarded to the upstream resolver where it could hang a
// parsing thread.
//
//   - F1: the QR flag must be clear. A response-shaped query (QR=1) can confuse
//     an upstream resolver into hanging or misbehaving, so it is rejected.
//   - F2: the answer/authority/additional counts must be consistent with the
//     bytes actually present. An inflated count (claims more records than the
//     message contains) is rejected before forwarding.
//   - F5: DNS names are validated for the RFC-1035 per-label (63) and
//     total-name (253) limits, and compression pointers are bounded.
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

	// F1: a query must have the QR flag clear. A response-shaped query
	// (QR=1) can confuse an upstream resolver into hanging or misbehaving.
	if header.Flags&0x8000 != 0 {
		return fmt.Errorf("query has QR flag set (response-shaped query rejected)")
	}

	// Check the header fields
	if header.QuestionCount == 0 {
		return fmt.Errorf("no questions in DNS request")
	}

	// Parse the questions.
	offset := 12
	for i := 0; i < int(header.QuestionCount); i++ {
		if _, err := parseName(v.Data, &offset); err != nil {
			return fmt.Errorf("failed to parse DNS question name: %v", err)
		}

		// Ensure the slice has enough capacity before reading the type and class
		if offset+4 > len(v.Data) {
			return fmt.Errorf("DNS request binary is too short")
		}
		// type (2) + class (2); the values are not semantically restricted here,
		// but the bounds guarantee the read is safe.
		offset += 4
	}

	// F2: validate the answer, authority, and additional sections. Every record
	// must fit within the message. A query that claims more records than the
	// bytes can contain (or is truncated) is rejected before it reaches the
	// upstream, where a parser waiting for records that never arrive would hang.
	sections := [3]uint16{header.AnswerCount, header.AuthorityCount, header.AdditionalCount}
	for _, count := range sections {
		for i := 0; i < int(count); i++ {
			if err := parseResourceRecord(v.Data, &offset); err != nil {
				return fmt.Errorf("failed to parse section record: %v", err)
			}
		}
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

// parseName reads a DNS name starting at *offset and advances *offset to the
// first byte after the name in the current record. It follows compression
// pointers (with a hop cap to prevent loops), enforces the RFC-1035 per-label
// limit (63) and total-name limit (253), and bounds every access against data.
//
// The returned offset is where parsing of the current record continues: past
// the terminating 0x00 for a plain name, or past the 2-byte pointer when the
// name ends with a compression pointer.
func parseName(data []byte, offset *int) (int, error) {
	const (
		maxLabelLen = 63
		maxNameLen  = 253
		maxHops     = 16
	)
	total := 0
	hops := 0
	// terminatorOffset is the offset the current record continues at when the
	// name ends via a compression pointer; -1 means the name ends via 0x00.
	terminatorOffset := -1
	for {
		if *offset >= len(data) {
			return 0, fmt.Errorf("name runs past end of message")
		}
		b := data[*offset]
		switch {
		case b == 0x00:
			// The record continues after the terminator 0x00 for a plain name,
			// or after the compression pointer when the name ended via a
			// pointer. Keep *offset equal to the returned offset so callers that
			// read *offset (not the return value) parse the next field correctly.
			if terminatorOffset >= 0 {
				*offset = terminatorOffset
			} else {
				*offset++
			}
			return *offset, nil
		case b&0xC0 == 0xC0:
			// Compression pointer: consumes this byte and the next.
			if *offset+1 >= len(data) {
				return 0, fmt.Errorf("truncated compression pointer")
			}
			hops++
			if hops > maxHops {
				return 0, fmt.Errorf("too many compression pointers (%d)", hops)
			}
			terminatorOffset = *offset + 2
			ptr := int(b&0x3F)<<8 | int(data[*offset+1])
			// A valid pointer targets an earlier location (names are written
			// left-to-right); reject forward/self pointers that could loop.
			if ptr >= *offset {
				return 0, fmt.Errorf("invalid compression pointer (offset %d -> %d)", *offset, ptr)
			}
			*offset = ptr
			continue
		default:
			labelLen := int(b)
			if labelLen > maxLabelLen {
				return 0, fmt.Errorf("label too long: %d (max %d)", labelLen, maxLabelLen)
			}
			if *offset+1+labelLen > len(data) {
				return 0, fmt.Errorf("label runs past end of message")
			}
			total += 1 + labelLen
			if total > maxNameLen {
				return 0, fmt.Errorf("name too long: %d (max %d)", total, maxNameLen)
			}
			*offset += 1 + labelLen
		}
	}
}

// parseResourceRecord reads one resource record (name + type + class + ttl +
// rdlength + rdata) starting at *offset and advances *offset past it, enforcing
// the RFC-1035 label/total-name limits (via parseName) and bounds on the fixed
// header and rdata length.
func parseResourceRecord(data []byte, offset *int) error {
	if _, err := parseName(data, offset); err != nil {
		return err
	}
	if *offset+10 > len(data) {
		return fmt.Errorf("resource record header runs past end of message")
	}
	// type(2) class(2) ttl(4) rdlength(2)
	rdlength := int(binary.BigEndian.Uint16(data[*offset+8 : *offset+10]))
	if *offset+10+rdlength > len(data) {
		return fmt.Errorf("resource record rdata runs past end of message")
	}
	*offset += 10 + rdlength
	return nil
}
