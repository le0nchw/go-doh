package handler

import (
	"bytes"
	"doh/internal/cache"
	"doh/internal/resolver"
	"doh/internal/startup"
	"doh/internal/util"
	"io"
	"log"
	"net/http"
	"sync"
)

var buffers = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Extract the DNS query from the request
	var dnsQuery []byte
	if r.Method == http.MethodGet {
		dnsQuery = extractDNSQueryFromGET(r)
	} else if r.Method == http.MethodPost {
		dnsQuery = extractDNSQueryFromPOST(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Make sure a DNS query is provided
	if dnsQuery == nil {
		http.Error(w, "No DNS query provided", http.StatusBadRequest)
		return
	}

	validator := util.NewDNSRequestValidator(dnsQuery)
	if err := validator.Validate(); err != nil {
		http.Error(w, "DNS query malformed", http.StatusBadRequest)
		log.Println("DNS query malformed:", err)
		return
	}

	dnsQueryString, _ := util.EncodeBase64URL(dnsQuery)
	if res, ok := cache.Get(dnsQueryString); ok {
		util.VLog("[hit] ", dnsQueryString)
		forwardDNSResponse(w, res)
		return
	}

	res := resolver.New(dnsQueryString)
	var response []byte
	var err error
	if *startup.Tcp {
		response, err = res.SendTcpQuery(dnsQuery)
	} else {
		response, err = res.SendUdpQuery(dnsQuery)
	}
	if err == resolver.ErrTcFlag {
		log.Println("[tc] ", dnsQueryString)
		response, err = res.SendTcpQuery(dnsQuery)
	}
	if err == resolver.ErrRCode {
		log.Println("[rc] ", dnsQueryString)
		response, err = res.SendTcpQuery(dnsQuery)
	}
	if err != nil {
		http.Error(w, "Failed to send DNS query", http.StatusInternalServerError)
		log.Println("Failed to send DNS query:", dnsQueryString, err)
		return
	}

	// Forward the DNS response to the client
	forwardDNSResponse(w, response)
}

// Extract the DNS query from the GET request
func extractDNSQueryFromGET(r *http.Request) []byte {
	dnsParam := r.URL.Query().Get("dns")
	if dnsParam != "" {
		dnsQuery, err := util.DecodeBase64URL(dnsParam)
		if err == nil {
			return dnsQuery
		}
	}
	return nil
}

// Extract the DNS query from the POST request
func extractDNSQueryFromPOST(w http.ResponseWriter, r *http.Request) []byte {
	// ponytail: DNS messages are <=65535 bytes over TCP; cap the body to
	// prevent unbounded memory growth (memory-exhaustion DoS).
	buffer := buffers.Get().(*bytes.Buffer)
	defer func() {
		buffer.Reset()
		buffers.Put(buffer)
	}()
	_, err := io.Copy(buffer, http.MaxBytesReader(w, r.Body, 65535))
	if err != nil {
		return nil
	}
	return buffer.Bytes()
}

// Forward the DNS response to the client
func forwardDNSResponse(w http.ResponseWriter, response []byte) {
	w.Header().Set("Content-Type", "application/dns-message")
	_, err := w.Write(response)
	if err != nil {
		log.Println("Failed to forward DNS response:", err)
	}
}
