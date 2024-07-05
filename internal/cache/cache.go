package cache

import (
	"doh/internal/startup"
	"sync"
	"time"
)

var data sync.Map

type dnsCache struct {
	Response []byte
	Expiry   time.Time
}

func init() {
	go func() {
		for range time.Tick(time.Minute) {
			data.Range(func(k, v any) bool {
				if v.(dnsCache).Expiry.Before(startup.CachedTime) {
					data.Delete(k)
				}
				return true
			})
		}
	}()
}

func Get(key string) ([]byte, bool) {

	if *startup.Ttl <= 0 {
		return nil, false
	}
	entry, ok := data.Load(key)
	if !ok || entry.(dnsCache).Expiry.Before(startup.CachedTime) {
		return nil, false
	}

	return entry.(dnsCache).Response, true
}

func Set(key string, response []byte, ttl time.Duration) {

	if *startup.Ttl <= 0 {
		return
	}
	data.Store(key, dnsCache{
		Response: response,
		Expiry:   startup.CachedTime.Add(ttl),
	})
}
