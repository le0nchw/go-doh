package resolver

import (
	"doh/internal/startup"
	"net"
)

// Send the UDP DNS query to upstream DNS resolver
func SendUdpQuery(query []byte) ([]byte, error) {
	conn, err := net.Dial("udp", *startup.Upstream)
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
