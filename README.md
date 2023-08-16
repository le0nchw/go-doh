## Go DOH Proxy
This is a DNS-over-HTTP(S) (DoH) proxy implemented in Go. It allows you to forward DNS queries over HTTP(S) to a upstream DNS resolver. The proxy acts as an intermediary between clients and the DNS resolver, providing a convenient way to make DNS queries over HTTP(S).


### Features
- Act as a proxy for your upstream DNS server while offering DoH functionality.
- Support DNS Wireformat ([RFC1035](https://datatracker.ietf.org/doc/html/rfc1035)).
- Efficiently caches DNS responses to alleviate upstream server load and optimize performance.
- Capable of integrating with Cloudflare Tunnel, NGINX, or other reverse proxy solutions to provide support for HTTPS requests.
- Capable of integrating with Pi-hole to enable DNS-over-HTTPS (DoH) functionality for your personal Pi-hole.


### Prerequisites
- Go 1.20 or higher
- Cloudflare account (optional, for Cloudflare Tunnel)
- NGINX installed and configured (optional, for reverse proxy)
- Pi-hole (optional, for local adblock DNS resolver)

### Installation
1. Clone the repository
```
git clone https://github.com/le0nchw/go-doh.git
```
```
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
cp init/go-doh.service /etc/systemd/system/go-doh.service
```
2. copy doh to /usr/local/bin
```
cp doh /usr/local/bin/doh
```
3. enable and start using systemctl
```
systemctl enable go-doh.service
```
```
systemctl start go-doh.service
```