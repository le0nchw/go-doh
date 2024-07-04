package resolver

import (
	"doh/internal/cache"
	"doh/internal/startup"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

var rs sync.Map

var connPool = sync.Pool{
	New: func() interface{} {
		conn, err := net.Dial("udp", *startup.Upstream)
		if err != nil {
			panic(err)
		}
		return conn
	},
}

var bytes = sync.Pool{
	New: func() interface{} {
		return make([]byte, 512)
	},
}

var tcpBytes = sync.Pool{
	New: func() interface{} {
		return make([]byte, 64000)
	},
}

func init() {
	go func() {
		for range time.Tick(time.Minute) {
			rs.Range(func(k, v any) bool {
				if v.(*Resolver).Expiry.Before(startup.CachedTime) {
					rs.Delete(k)
				}
				return true
			})
		}
	}()
}

type Resolver struct {
	lock   sync.Mutex
	Expiry time.Time
}

func New(key string) *Resolver {
	if val, ok := rs.Load(key); ok {
		return val.(*Resolver)
	}
	r := Resolver{
		Expiry: startup.CachedTime.Add(time.Second * 10),
	}
	rs.Store(key, &r)
	return &r
}

// Send the UDP DNS query to upstream DNS resolver
func (r *Resolver) SendUdpQuery(query []byte) ([]byte, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	queryString := string(query)
	if res, ok := cache.Get(queryString); ok {
		return res, nil
	}
	conn := connPool.Get().(net.Conn)
	defer connPool.Put(conn)

	_, err := conn.Write(query)
	if err != nil {
		return nil, err
	}

	buffer := bytes.Get().([]byte)
	defer bytes.Put(buffer)
	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}

	// Resend the query using TCP if TC flag is set
	if isTruncated(buffer[:n]) {
		buffer, err = r.SendTcpQuery(query)
	}
	if err != nil {
		return nil, err
	}

	cache.Set(queryString, buffer[:n], time.Second*time.Duration(*startup.Ttl))
	return buffer[:n], nil
}

// Send the TCP DNS query to upstream DNS resolver
func (r *Resolver) SendTcpQuery(query []byte) ([]byte, error) {
	r.lock.Lock()
	defer r.lock.Unlock()
	queryString := string(query)
	if res, ok := cache.Get(queryString); ok {
		return res, nil
	}

	conn, err := net.Dial("tcp", *startup.Upstream)
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}()

	if err != nil {
		return nil, err
	}

	// Write the length of the query as a 2-byte big-endian integer
	var length [2]byte
	binary.BigEndian.PutUint16(length[:], uint16(len(query)))
	_, err = conn.Write(length[:])
	if err != nil {
		return nil, err
	}

	// Write the query
	_, err = conn.Write(query)
	if err != nil {
		return nil, err
	}

	// Read the length of the response
	_, err = io.ReadFull(conn, length[:])
	if err != nil {
		return nil, err
	}
	responseLength := binary.BigEndian.Uint16(length[:])

	// Read the response
	buffer := tcpBytes.Get().([]byte)
	defer tcpBytes.Put(buffer)
	n, err := io.ReadFull(conn, buffer[:responseLength])
	if err != nil {
		return nil, err
	}

	cache.Set(queryString, buffer[:n], time.Second*time.Duration(*startup.Ttl))
	return buffer[:n], nil
}

func isTruncated(resp []byte) bool {
	return (resp[2]&0x02 != 0)
}
