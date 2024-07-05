package startup

import (
	"flag"
	"time"
)

var Upstream = flag.String("upstream", "127.0.0.1:53", "DNS Upstream Address and Port")
var Port = flag.Int("port", 8080, "Port to listen")
var Ttl = flag.Int("ttl", 30, "Time to cache in seconds")
var Tcp = flag.Bool("tcp", false, "Use TCP to connect DNS upstream")

var CachedTime = time.Now()

func init() {
	flag.Parse()
	go updateCachedTime()
}

func updateCachedTime() {
	for range time.Tick(time.Second) {
		CachedTime = time.Now()
	}
}
