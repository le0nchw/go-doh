package cache

import (
	"sync"
	"time"
)

// DNSCacheEntry represents a cached DNS response
type DNSCacheEntry struct {
	Response []byte
	Expiry   time.Time
}

type DNSCache struct {
	cache map[string]DNSCacheEntry
	mutex sync.RWMutex
}

func NewDNSCache() *DNSCache {
	c := DNSCache{
		cache: make(map[string]DNSCacheEntry),
	}
	go func() {
		for range time.Tick(time.Minute) {
			for k, v := range c.cache {
				if v.Expiry.Before(time.Now()) {
					c.mutex.Lock()
					delete(c.cache, k)
					c.mutex.Unlock()
				}
			}
		}
	}()
	return &c
}

func (c *DNSCache) Get(key string) ([]byte, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, ok := c.cache[key]
	if !ok || entry.Expiry.Before(time.Now()) {
		return nil, false
	}

	return entry.Response, true
}

func (c *DNSCache) Set(key string, response []byte, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache[key] = DNSCacheEntry{
		Response: response,
		Expiry:   time.Now().Add(ttl),
	}
}

var Cache *DNSCache
