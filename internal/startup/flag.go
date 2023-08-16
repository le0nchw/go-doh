package startup

import "flag"

var Upstream = flag.String("upstream", "127.0.0.1:53", "DNS Upstream Address and Port")
var Port = flag.Int("port", 8080, "Port to listen")
var Ttl = flag.Int("ttl", 30, "Time to cache in seconds")

func init() {
	flag.Parse()
}
