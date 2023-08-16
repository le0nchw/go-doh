## Go DOH Proxy
This is a DNS-over-HTTP(S) (DoH) proxy implemented in Go. It allows you to forward DNS queries over HTTP to a upstream DNS resolver. The proxy acts as an intermediary between clients and the DNS resolver, providing a convenient way to make DNS queries over HTTP(S).


### Features
- Listens on a specified port for incoming HTTP requests.
- Support DOH Wireformat ([RFC1035](https://datatracker.ietf.org/doc/html/rfc1035)).
- Caches DNS responses to improve performance.
- Forwards the DNS query to upstream server.
- Returns the DNS response to the client.
- Integrates with Cloudflare Argo Tunnel, NGINX or other reverse proxy to support HTTPS request.
- Integrates with pihole


### Prerequisites
- Go 1.20 or higher
- Cloudflare account (optional, for Argo Tunnel)
- NGINX installed and configured (optional, for reverse proxy)
- Pi-hole (optional, for local adblock DNS resolver)

### Installation
1. Clone the repository
```
git clone https://github.com/le0nchw/go-doh

cd go-doh
```
2. Build the executable
```
bash build.sh
```

### Usage
The proxy can be configured by passing command-line arguments:
```
./doh [-upstream <address:port>] [-port <port>] [-ttl <ttl>]
```
- upstream: Specifies the address and port of the upstream DNS server. The default is 127.0.0.1:53.
- port: Specifies the port to listen on for incoming HTTP requests. The default is 8080.
- ttl : Specifies the TTL for the cache in seconds. The default is 30.

### Run as Service Using systemd
1. copy init/go-doh.service to /etc/systemd/system
```
cp /etc/init/go-doh.service /etc/systemd/system
```
2. copy doh to /usr/local/bin
```
cp doh /usr/local/bin/doh
```
3. enable and start using systemctl
```
systemctl enable go-doh.service
systemctl start go-doh.service
```