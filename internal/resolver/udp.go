package resolver

import (
	"doh/internal/cache"
	"doh/internal/startup"
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

func init() {
	go func() {
		for range time.Tick(time.Minute) {
			rs.Range(func(k, v any) bool {
				if v.(Resolver).Expiry.Before(time.Now()) {
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
		val.(*Resolver).Expiry = time.Now().Add(time.Second * 10)
		return val.(*Resolver)
	} else {
		val := &Resolver{
			Expiry: time.Now().Add(time.Second * 10),
		}
		return val
	}
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

	buffer := make([]byte, 512)
	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	buffer = buffer[:n]

	cache.Set(queryString, buffer, time.Second*time.Duration(*startup.Ttl))
	return buffer, nil
}
