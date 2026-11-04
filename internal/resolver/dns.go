package resolver

import (
	"doh/internal/cache"
	"doh/internal/startup"
	"doh/internal/util"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

var ErrTcFlag error = errors.New("TC")
var ErrRCode error = errors.New("RC")

var rs sync.Map

var connPool = sync.Pool{
	New: func() interface{} {
		conn, err := net.Dial("udp", *startup.Upstream)
		if err != nil {
			// ponytail: return a conn that fails on use instead of panicking
			// the caller (F8); SendUdpQuery already propagates the error.
			return &errConn{err: err}
		}
		return conn
	},
}

// errConn is a placeholder net.Conn used when the upstream can't be dialed.
type errConn struct{ err error }

func (c *errConn) Read(_ []byte) (int, error)     { return 0, c.err }
func (c *errConn) Write(_ []byte) (int, error)    { return 0, c.err }
func (c *errConn) Close() error                   { return nil }
func (c *errConn) LocalAddr() net.Addr            { return nil }
func (c *errConn) RemoteAddr() net.Addr           { return nil }
func (c *errConn) SetDeadline(_ time.Time) error  { return nil }
func (c *errConn) SetReadDeadline(_ time.Time) error { return nil }
func (c *errConn) SetWriteDeadline(_ time.Time) error { return nil }

var bytes = sync.Pool{
	New: func() interface{} {
		return make([]byte, 512)
	},
}

var tcpBytes = sync.Pool{
	New: func() interface{} {
		// ponytail: DNS-over-TCP frames are up to 65535 bytes (uint16 max);
		// size the buffer to the max so responseLength can never slice out of
		// range and panic (F4).
		return make([]byte, 65535)
	},
}

func init() {
	go func() {
		for range time.Tick(time.Minute) {
			rs.Range(func(k, v any) bool {
				r := v.(*Resolver)
				r.lock.Lock()
				expiry := r.Expiry
				r.lock.Unlock()
				if expiry.Before(startup.Now()) {
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
	r := &Resolver{}
	r.lock.Lock()
	r.Expiry = startup.Now().Add(time.Second * 10)
	r.lock.Unlock()
	rs.Store(key, r)
	return r
}

// Send the UDP DNS query to upstream DNS resolver
func (r *Resolver) SendUdpQuery(query []byte) ([]byte, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	domain := util.GetDomainName(query)
	queryString, _ := util.EncodeBase64URL(query)
	if res, ok := cache.Get(queryString); ok {
		debugStr, _ := util.EncodeBase64URL(res)
		util.VLog("[hit] ", domain, queryString, debugStr)
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

	// ponytail: copy out of the pooled buffer before returning/caching so the
	// backing array can be safely handed back to the pool (data-integrity fix).
	resp := make([]byte, n)
	copy(resp, buffer[:n])

	// Resend the query using TCP if TC flag is set
	if isTruncated(resp) {
		return nil, ErrTcFlag
	}
	if isDNSError(getRCode(resp)) {
		return nil, ErrRCode
	}
	util.VLog("[reply] ", domain)
	cache.Set(queryString, resp, time.Second*time.Duration(*startup.Ttl))
	return resp, nil
}

// Send the TCP DNS query to upstream DNS resolver
func (r *Resolver) SendTcpQuery(query []byte) ([]byte, error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	domain := util.GetDomainName(query)
	queryString, _ := util.EncodeBase64URL(query)

	if res, ok := cache.Get(queryString); ok {
		debugStr, _ := util.EncodeBase64URL(res)
		util.VLog("[hit] ", domain, queryString, debugStr)
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

	// ponytail: copy out of the pooled buffer before returning/caching so the
	// backing array can be safely handed back to the pool (data-integrity fix).
	resp := make([]byte, n)
	copy(resp, buffer[:n])

	if isDNSError(getRCode(resp)) {
		return nil, ErrRCode
	}

	util.VLog("[reply] ", domain)
	cache.Set(queryString, resp, time.Second*time.Duration(*startup.Ttl))
	return resp, nil
}

func isTruncated(resp []byte) bool {
	if len(resp) < 4 {
		return false
	}
	// ponytail: TC is bit 9 of the flags field = 0x02 in byte 2, not 0x04
	// (that's the AA flag). The old mask flagged every authoritative reply as
	// truncated and forced an unnecessary TCP fallback.
	return resp[2]&0x02 != 0
}

func getRCode(resp []byte) uint8 {
	if len(resp) < 4 {
		return 0
	}
	// ponytail: RCODE is the low nibble of byte 3 (0x0F), not the high nibble.
	// The high nibble holds RA/Z/NS/CD flags; a normal reply sets RA=1 (0x80),
	// which the old high-nibble read misread as RCODE=8 and rejected every
	// successful query as "RC".
	return resp[3] & 0x0F
}

func isDNSError(rcode uint8) bool {
	switch rcode {
	case 0:
		return false
	case 3:
		return false
	default:
		return true
	}
}
