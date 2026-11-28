package cache

import (
	"doh/internal/startup"
	"sync"
	"time"
)

// maxEntries bounds the number of cached responses to prevent unbounded memory
// growth when caching is enabled (F4). Without a cap, an attacker issuing many
// unique queries can grow the map without limit and exhaust the host's memory.
// When the limit is reached the oldest entry is evicted before a new one is
// inserted.
const maxEntries = 4096

type dnsCache struct {
	Response []byte
	Expiry   time.Time
}

var (
	mu    sync.Mutex
	data  = make(map[string]dnsCache, maxEntries)
	order []string // insertion order, used to evict the oldest entry
)

// prune removes expired entries. Caller must hold mu.
func prune() {
	for i := 0; i < len(order); i++ {
		key := order[i]
		entry, ok := data[key]
		if !ok {
			// Already evicted; drop the stale order entry.
			order = append(order[:i], order[i+1:]...)
			i--
			continue
		}
		if entry.Expiry.Before(startup.Now()) {
			delete(data, key)
			order = append(order[:i], order[i+1:]...)
			i--
		}
	}
}

func Get(key string) ([]byte, bool) {

	if *startup.Ttl <= 0 {
		return nil, false
	}
	mu.Lock()
	defer mu.Unlock()
	entry, ok := data[key]
	if !ok || entry.Expiry.Before(startup.Now()) {
		return nil, false
	}

	return entry.Response, true
}

func Set(key string, response []byte, ttl time.Duration) {

	if *startup.Ttl <= 0 {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	// Reclaim expired entries so the map does not grow past maxEntries even if
	// some keys are never re-accessed, then evict the oldest entry if still at
	// capacity.
	prune()
	if len(data) >= maxEntries && len(order) > 0 {
		oldest := order[0]
		delete(data, oldest)
		order = order[1:]
	}
	// Mark this key as the most-recently-inserted.
	for i := range order {
		if order[i] == key {
			order = append(order[:i], order[i+1:]...)
			break
		}
	}
	data[key] = dnsCache{
		Response: response,
		Expiry:   startup.Now().Add(ttl),
	}
	order = append(order, key)
}
