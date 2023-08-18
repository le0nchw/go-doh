package cache

import (
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
				if v.(dnsCache).Expiry.Before(time.Now()) {
					data.Delete(k)
				}
				return true
			})
		}
	}()
}

func Get(key string) ([]byte, bool) {

	entry, ok := data.Load(key)
	if !ok || entry.(dnsCache).Expiry.Before(time.Now()) {
		return nil, false
	}

	return entry.(dnsCache).Response, true
}

func Set(key string, response []byte, ttl time.Duration) {

	data.Store(key, dnsCache{
		Response: response,
		Expiry:   time.Now().Add(ttl),
	})
}
