package util

import "testing"

// TestEncodingRoundTrip validates F12: Encode/Decode use a consistent,
// URL-safe encoding and round-trip.
func TestEncodingRoundTrip(t *testing.T) {
	in := []byte{0x12, 0x34, 0xff, 0x00, 0x2b, 0x2f, 0x3d}
	enc, err := EncodeBase64URL(in)
	if err != nil {
		t.Fatalf("EncodeBase64URL: %v", err)
	}
	// URL-safe encoding must not contain +, /, or =.
	for _, r := range enc {
		if r == '+' || r == '/' || r == '=' {
			t.Fatalf("encoding is not URL-safe: %q", enc)
		}
	}
	out, err := DecodeBase64URL(enc)
	if err != nil {
		t.Fatalf("DecodeBase64URL: %v", err)
	}
	if string(out) != string(in) {
		t.Fatalf("round-trip mismatch: got %v want %v", out, in)
	}
}

// TestGetDomainNameNoPanic validates F7: crafted/short input must not panic.
func TestGetDomainNameNoPanic(t *testing.T) {
	cases := [][]byte{nil, {}, {1, 2, 3}, {0x00, 0x01}, make([]byte, 12)}
	for _, c := range cases {
		_ = GetDomainName(c) // must not panic
	}
	// A label that claims more bytes than remain must stop cleanly.
	crafted := make([]byte, 14) // 12-byte header + 2 bytes
	crafted[12] = 5             // claims a 5-byte label, only 2 bytes remain
	_ = GetDomainName(crafted)
}
