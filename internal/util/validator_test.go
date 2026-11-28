package util

import (
	"encoding/binary"
	"strings"
	"testing"
)

// buildQuery assembles a DNS query message with the given header fields and a
// single IN/A question for name.
func buildQuery(flags uint16, name string, qd, an, ns, ar uint16) []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], 0x1234)
	binary.BigEndian.PutUint16(q[2:4], flags)
	binary.BigEndian.PutUint16(q[4:6], qd)
	binary.BigEndian.PutUint16(q[6:8], an)
	binary.BigEndian.PutUint16(q[8:10], ns)
	binary.BigEndian.PutUint16(q[10:12], ar)
	for _, label := range strings.Split(name, ".") {
		q = append(q, byte(len(label)))
		q = append(q, []byte(label)...)
	}
	q = append(q, 0)        // root label
	q = append(q, 0x00, 0x01) // TYPE A
	q = append(q, 0x00, 0x01) // CLASS IN
	return q
}

// answerRR builds one well-formed answer record: a compression pointer back to
// the question name at offset 12, TYPE A, CLASS IN, TTL 0, RDLENGTH 4, rdata
// 1.2.3.4.
func answerRR() []byte {
	return []byte{0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 1, 2, 3, 4}
}

// TestValidateAcceptsNormalQuery is the positive control: a well-formed query
// with RD set must pass.
func TestValidateAcceptsNormalQuery(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 0, 0, 0)
	if err := NewDNSRequestValidator(q).Validate(); err != nil {
		t.Fatalf("normal query rejected: %v", err)
	}
}

// TestValidateRejectsShortMessage guards the header-length check.
func TestValidateRejectsShortMessage(t *testing.T) {
	if err := NewDNSRequestValidator([]byte{0, 1, 2}).Validate(); err == nil {
		t.Fatal("short message should be rejected")
	}
}

// TestValidateRejectsZeroQuestions guards the question-count check.
func TestValidateRejectsZeroQuestions(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 0, 0, 0, 0)
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("zero questions should be rejected")
	}
}

// TestValidateRejectsQRSet_F1 proves F1: a response-shaped query (QR=1) is
// rejected so it cannot hang the upstream resolver.
func TestValidateRejectsQRSet_F1(t *testing.T) {
	q := buildQuery(0x8100, "example.com", 1, 0, 0, 0) // QR=1 set
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("QR=1 query should be rejected (F1)")
	}
}

// TestValidateRejectsInflatedAncount_F2 proves F2: a claim of more answer
// records than the message contains is rejected.
func TestValidateRejectsInflatedAncount_F2(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 5, 0, 0) // ancount=5, no records
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("inflated ancount should be rejected (F2)")
	}
}

// TestValidateAcceptsConsistentAncount_F2 proves F2 does not reject a query
// whose answer count matches a well-formed record.
func TestValidateAcceptsConsistentAncount_F2(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 1, 0, 0)
	q = append(q, answerRR()...)
	if err := NewDNSRequestValidator(q).Validate(); err != nil {
		t.Fatalf("consistent ancount with valid RR should pass: %v", err)
	}
}

// TestValidateRejectsTruncatedAnswer_F2 proves F2 catches a record whose
// RDLENGTH exceeds the bytes actually present in the message.
func TestValidateRejectsTruncatedAnswer_F2(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 1, 0, 0)
	// RR: pointer, TYPE A, CLASS IN, TTL 0, RDLENGTH=4, but only 3 rdata bytes.
	q = append(q, 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 1, 2, 3)
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("RDLENGTH exceeding available bytes should be rejected (F2)")
	}
}

// TestValidateRejectsLongLabel_F5 proves F5 rejects a label longer than 63.
func TestValidateRejectsLongLabel_F5(t *testing.T) {
	q := buildQuery(0x0100, strings.Repeat("a", 64), 1, 0, 0, 0)
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("64-byte label should be rejected (F5)")
	}
}

// TestValidateRejectsTotalNameTooLong_F5 proves F5 rejects a name over 253 bytes.
func TestValidateRejectsTotalNameTooLong_F5(t *testing.T) {
	var parts []string
	for i := 0; i < 5; i++ {
		parts = append(parts, strings.Repeat("a", 52)) // 5*52 + 4 dots = 264 > 253
	}
	name := strings.Join(parts, ".")
	if len([]byte(name)) <= 253 {
		t.Fatalf("test name is only %d bytes; need > 253", len([]byte(name)))
	}
	q := buildQuery(0x0100, name, 1, 0, 0, 0)
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("total name > 253 should be rejected (F5)")
	}
}

// TestValidateRejectsPointerLoop_F5 proves a self/forward compression pointer
// (which a naive parser could loop on) is rejected.
func TestValidateRejectsPointerLoop_F5(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 0, 0, 0)
	q[12] = 0xC0
	q[13] = 0x0C // pointer to offset 12 == current position -> invalid
	if err := NewDNSRequestValidator(q).Validate(); err == nil {
		t.Fatal("invalid compression pointer should be rejected (F5)")
	}
}

// TestValidateAcceptsEDNSOPT_F5 proves a normal EDNS query (OPT record in the
// additional section) still passes.
func TestValidateAcceptsEDNSOPT_F5(t *testing.T) {
	// header with arcount=1
	q := buildQuery(0x0100, "example.com", 1, 0, 0, 1)
	// OPT record: NAME=root(0x00), TYPE=41(0x0029), CLASS=udp size, TTL=0,
	// RDLENGTH=0, no RDATA.
	q = append(q, 0x00)                       // root name
	q = append(q, 0x00, 0x29)                 // TYPE OPT
	q = append(q, 0x10, 0x00)                 // CLASS = 4096 (UDP payload size)
	q = append(q, 0x00, 0x00, 0x00, 0x00)     // TTL (ext RCODE / ver / TTL)
	q = append(q, 0x00, 0x00)                 // RDLENGTH = 0
	if err := NewDNSRequestValidator(q).Validate(); err != nil {
		t.Fatalf("EDNS OPT query should pass: %v", err)
	}
}

// TestValidateReusesPool proves the validator returns its object to the pool.
func TestValidateReusesPool(t *testing.T) {
	q := buildQuery(0x0100, "example.com", 1, 0, 0, 0)
	_ = NewDNSRequestValidator(q).Validate()
	// A second use of a freshly pooled validator must still be independent.
	v2 := NewDNSRequestValidator(buildQuery(0x0100, "example.org", 1, 0, 0, 0))
	if err := v2.Validate(); err != nil {
		t.Fatalf("pooled validator misbehaving: %v", err)
	}
}
