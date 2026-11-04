package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestExtractDNSQueryFromPOSTBounds validates F1: an oversized POST body must
// yield no query (nil) rather than being streamed into memory unbounded.
func TestExtractDNSQueryFromPOSTBounds(t *testing.T) {
	large := make([]byte, 70000) // > 65535 cap
	for i := range large {
		large[i] = 'a'
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(large))
	w := httptest.NewRecorder()
	if got := extractDNSQueryFromPOST(w, req); got != nil {
		t.Fatalf("expected nil for oversized body, got %d bytes", len(got))
	}
}
