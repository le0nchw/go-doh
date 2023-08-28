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

var bytes = sync.Pool{
	New: func() interface{} {
		return make([]byte, 512)
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
	n, err := conn.Read(buffer)
	if err != nil {
		return nil, err
	}
	buff := buffer[:n]
	bytes.Put(buffer)

	cache.Set(queryString, buff, time.Second*time.Duration(*startup.Ttl))
	return buff, nil
}
