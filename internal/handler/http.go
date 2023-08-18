package handler

import (
	"bytes"
	"doh/internal/cache"
	"doh/internal/resolver"
	"doh/internal/util"
	"io"
	"log"
	"net/http"
)

func HandleRequest(w http.ResponseWriter, r *http.Request) {
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

	res := resolver.New(dnsQueryString)
	response, err := res.SendUdpQuery(dnsQuery)
	if err != nil {
		http.Error(w, "Failed to send DNS query", http.StatusInternalServerError)
		log.Println("Failed to send DNS query:", err)
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
func extractDNSQueryFromPOST(r *http.Request) []byte {
	buffer := new(bytes.Buffer)
	_, err := io.Copy(buffer, r.Body)
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
