package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var upstream = flag.String("upstream", "127.0.0.1:53", "DNS Upstream Address and Port")
var port = flag.Int("port", 8080, "Port to listen")

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

func main() {
	flag.Parse()

	// Create a new HTTP server
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", *port),
	}

	cache := NewDNSCache()

	// Handle incoming HTTP requests
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Extract the DNS query from the request
		var dnsQuery []byte
		if r.Method == http.MethodGet {
			dnsQuery = extractDNSQueryFromGET(r)
		} else if r.Method == http.MethodPost {
			dnsQuery = extractDNSQueryFromPOST(r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Make sure a DNS query is provided
		if dnsQuery == nil {
			http.Error(w, "No DNS query provided", http.StatusBadRequest)
			return
		}
		dnsQueryString := string(dnsQuery)
		if res, ok := cache.Get(dnsQueryString); ok {
			forwardDNSResponse(w, res)
			return
		}

		// Send the DNS query to the local DNS resolver
		response, err := sendDNSQuery(dnsQuery)
		if err != nil {
			http.Error(w, "Failed to send DNS query", http.StatusInternalServerError)
			log.Println("Failed to send DNS query:", err)
			return
		}

		// Forward the DNS response to the client
		go cache.Set(dnsQueryString, response, time.Second*30)
		forwardDNSResponse(w, response)
	})

	c := make(chan os.Signal, 1)
	defer close(c)
	signal.Notify(c, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Start the HTTP server
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()
	log.Printf("DoH proxy started on http://localhost:%d, upstream server: %s\n", *port, *upstream)

	<-c
	log.Print("Graceful shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed:%+v", err)
	}
	log.Print("Server Exited")
	os.Exit(0)
}

// Extract the DNS query from the GET request
func extractDNSQueryFromGET(r *http.Request) []byte {
	dnsParam := r.URL.Query().Get("dns")
	if dnsParam != "" {
		dnsQuery, err := decodeBase64URL(dnsParam)
		if err == nil {
			return dnsQuery
		}
	}
	return nil
}

// Extract the DNS query from the POST request
func extractDNSQueryFromPOST(r *http.Request) []byte {
	buffer := new(bytes.Buffer)
	_, err := io.Copy(buffer, r.Body)
	if err != nil {
		return nil
	}
	return buffer.Bytes()
}

// Decode base64 URL-encoded data
func decodeBase64URL(data string) ([]byte, error) {
	// Convert base64 URL encoded string to standard base64 encoding
	data = strings.ReplaceAll(data, "-", "+")
	data = strings.ReplaceAll(data, "_", "/")

	// Add padding if necessary
	pad := len(data) % 4
	if pad != 0 {
		data += strings.Repeat("=", 4-pad)
	}

	// Decode base64 data
	return base64.StdEncoding.DecodeString(data)
}

// Send the DNS query to the local DNS resolver
func sendDNSQuery(query []byte) ([]byte, error) {
	conn, err := net.Dial("udp", *upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_, err = conn.Write(query)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, 512)
	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	buffer = buffer[:n]

	return buffer, nil
}

// Forward the DNS response to the client
func forwardDNSResponse(w http.ResponseWriter, response []byte) {
	w.Header().Set("Content-Type", "application/dns-message")
	_, err := w.Write(response)
	if err != nil {
		log.Println("Failed to forward DNS response:", err)
	}
}
