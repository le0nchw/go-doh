package startup

import (
	"flag"
	"sync/atomic"
	"time"
)

var Upstream = flag.String("upstream", "127.0.0.1:53", "DNS Upstream Address and Port")
var Port = flag.Int("port", 8080, "Port to listen")
var BindAddress = flag.String("bind", "127.0.0.1", "Address to bind the HTTP server")
var Ttl = flag.Int("ttl", 0, "Time to cache in seconds")
var Tcp = flag.Bool("tcp", false, "Use TCP to connect DNS upstream")
var Verbose = flag.Bool("v", false, "Enable verbose logging")

// ponytail: atomic int64 of unix seconds so CachedTime can be read from
// handler/resolver/cache goroutines without a data race (F9). The ticker
// updates once per second, so second precision is sufficient.
var CachedTime atomic.Int64

// Now returns the current cached wall-clock time.
func Now() time.Time { return time.Unix(CachedTime.Load(), 0) }

func init() {
	CachedTime.Store(time.Now().Unix())
	go updateCachedTime()
}

func updateCachedTime() {
	for range time.Tick(time.Second) {
		CachedTime.Store(time.Now().Unix())
	}
}
