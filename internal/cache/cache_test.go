package cache

import (
	"fmt"
	"testing"
	"time"

	"doh/internal/startup"
)

// resetCacheForTest isolates this test from the shared package state and
// restores it afterwards.
func resetCacheForTest(t *testing.T) {
	t.Helper()
	mu.Lock()
	data = make(map[string]dnsCache, maxEntries)
	order = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		data = make(map[string]dnsCache, maxEntries)
		order = nil
		mu.Unlock()
	})
}

// TestCacheBoundedByMaxEntries_F4 proves F4: the cache never grows past
// maxEntries even when far more than that many unique keys are inserted.
func TestCacheBoundedByMaxEntries_F4(t *testing.T) {
	resetCacheForTest(t)

	*startup.Ttl = 300
	defer func() { *startup.Ttl = 0 }()

	n := maxEntries * 3
	for i := 0; i < n; i++ {
		Set(fmt.Sprintf("key-%d", i), []byte{byte(i)}, time.Minute)
	}

	mu.Lock()
	got := len(data)
	mu.Unlock()
	if got > maxEntries {
		t.Fatalf("cache grew beyond maxEntries: got %d, want <= %d", got, maxEntries)
	}
	// The most recent insert must survive; the oldest must have been evicted.
	if _, ok := data[fmt.Sprintf("key-%d", n-1)]; !ok {
		t.Error("newest entry was evicted (cache is not LRU-ish)")
	}
	if _, ok := data["key-0"]; ok {
		t.Error("oldest entry was not evicted")
	}
}

// TestCacheGetSetRoundTrip proves basic caching still works when enabled.
func TestCacheGetSetRoundTrip(t *testing.T) {
	resetCacheForTest(t)

	*startup.Ttl = 300
	defer func() { *startup.Ttl = 0 }()

	Set("k", []byte("v"), time.Minute)
	got, ok := Get("k")
	if !ok {
		t.Fatal("expected key to be cached")
	}
	if string(got) != "v" {
		t.Fatalf("cached value mismatch: got %q want %q", got, "v")
	}
}

// TestCacheTtlZeroDisablesCaching proves the no-cache path is unchanged.
func TestCacheTtlZeroDisablesCaching(t *testing.T) {
	resetCacheForTest(t)

	*startup.Ttl = 0
	Set("k", []byte("v"), time.Minute)
	if _, ok := Get("k"); ok {
		t.Fatal("with Ttl=0, Get must return not-found")
	}
}

// TestCacheExpiredEntryProven proves expired entries are not returned.
//
// The cache's time source is startup.Now(), which is second-precision, so we
// place an entry with an expiry already in the past rather than relying on real
// time to elapse.
func TestCacheExpiredEntryProven(t *testing.T) {
	resetCacheForTest(t)

	*startup.Ttl = 300
	defer func() { *startup.Ttl = 0 }()

	mu.Lock()
	data["k"] = dnsCache{Response: []byte("v"), Expiry: startup.Now().Add(-time.Hour)}
	mu.Unlock()

	if _, ok := Get("k"); ok {
		t.Fatal("expired entry should not be returned")
	}
}
