package resolver

import (
	stdbytes "bytes"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"doh/internal/startup"
)

// TestFlagReadsNoPanic validates F6: reading response flags on short input
// must not panic and must behave sanely.
func TestFlagReadsNoPanic(t *testing.T) {
	if isTruncated([]byte{}) {
		t.Error("isTruncated([]) should be false")
	}
	if isTruncated([]byte{0, 0, 0}) { // len 3, < 4
		t.Error("isTruncated(len 3) should be false")
	}
	if getRCode([]byte{}) != 0 {
		t.Error("getRCode([]) should be 0")
	}
	// Minimum 4-byte header: RCODE is the low nibble of byte 3.
	if getRCode([]byte{0, 0, 0, 0x05}) != 5 {
		t.Error("getRCode should return 5")
	}
	if !isTruncated([]byte{0, 0, 0x02, 0x00}) {
		t.Error("isTruncated should detect the TC flag")
	}
}

// TestFlagReadsCorrectBits proves the flag bits are read from the right
// positions: TC is bit 9 of byte 2 (0x02), RCODE is the low nibble of byte 3
// (0x0F). This is the regression test for the production "RC" failure where
// RA=1 (0x80) in byte 3 was misread as RCODE=8 and every successful reply was
// rejected.
func TestFlagReadsCorrectBits(t *testing.T) {
	// TC set (0x02), AA clear (0x04) -> truncated.
	if !isTruncated([]byte{0, 0, 0x02, 0x00}) {
		t.Error("TC flag (0x02) should be detected as truncated")
	}
	// AA set (0x04), TC clear (0x02) -> not truncated.
	if isTruncated([]byte{0, 0, 0x04, 0x00}) {
		t.Error("AA flag (0x04) must not be read as the TC flag")
	}
	// RCODE 5 (REFUSED) in the low nibble of byte 3.
	if got := getRCode([]byte{0, 0, 0x00, 0x05}); got != 5 {
		t.Errorf("getRCode low nibble: got %d want 5", got)
	}
	// RA set (0x80), RCODE 0 in the low nibble -> No Error.
	if got := getRCode([]byte{0, 0, 0x00, 0x80}); got != 0 {
		t.Errorf("getRCode must ignore the RA flag: got %d want 0", got)
	}
}

// TestResponseWithRAFlagNotError reproduces the production bug end to end: a
// live upstream returns a normal reply with RA=1 (0x80 in byte 3) and RCODE 0.
// The resolver must treat it as success, not as ErrRCode.
func TestResponseWithRAFlagNotError(t *testing.T) {
	upstream, cleanup := startUDPServer(t)
	defer cleanup()
	setUpstream(t, upstream)
	resetConnPool()
	*startup.Ttl = 0

	r := New("ra-flag")
	resp, err := r.SendUdpQuery(makeDNSQuery(1, "example.com"))
	if err != nil {
		t.Fatalf("SendUdpQuery with RA flag set: %v", err)
	}
	if len(resp) < 12 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}
	if resp[3]&0x80 == 0 {
		t.Error("test server did not set RA=1 as expected")
	}
}

// --- test helpers ---------------------------------------------------------

// makeDNSQuery builds a minimal but well-formed DNS query for the given name.
func makeDNSQuery(id uint16, domain string) []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], id)
	binary.BigEndian.PutUint16(q[2:4], 0x0100) // RD=1, QDCOUNT=1
	binary.BigEndian.PutUint16(q[4:6], 1)      // QDCOUNT = 1
	for _, label := range strings.Split(domain, ".") {
		q = append(q, byte(len(label)))
		q = append(q, []byte(label)...)
	}
	q = append(q, 0)         // root label
	q = append(q, 0x00, 0x01) // TYPE A
	q = append(q, 0x00, 0x01) // CLASS IN
	return q
}

// buildResponse echoes the query back as a valid DNS reply: QR=1, TC=0, RCODE=0.
// resp[11] carries the query ID low byte so distinct queries yield distinct
// replies, which lets the integrity test detect buffer clobbering.
func buildResponse(query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = 0x81 // QR=1, RD echoed
	resp[3] = 0x80 // RA=1, RCODE 0  (ponytail: realistic reply; RA set is what
	               // tripped the old high-nibble RCODE read into "RC" failures
	resp[11] = query[1]
	return resp
}

func startUDPServer(t *testing.T) (string, func()) {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			resp := buildResponse(buf[:n])
			if _, err := conn.WriteToUDP(resp, raddr); err != nil {
				return
			}
		}
	}()
	return conn.LocalAddr().String(), func() { conn.Close() }
}

func startTCPServer(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go handleTCP(c)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

// handleTCP speaks the length-prefixed DNS-over-TCP framing the resolver uses.
func handleTCP(conn net.Conn) {
	defer conn.Close()
	var lenBuf [2]byte
	for {
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(lenBuf[:])
		query := make([]byte, n)
		if _, err := io.ReadFull(conn, query); err != nil {
			return
		}
		resp := buildResponse(query)
		var out [2]byte
		binary.BigEndian.PutUint16(out[:], uint16(len(resp)))
		if _, err := conn.Write(append(out[:], resp...)); err != nil {
			return
		}
	}
}

func setUpstream(t *testing.T, addr string) {
	t.Helper()
	old := *startup.Upstream
	*startup.Upstream = addr
	t.Cleanup(func() { *startup.Upstream = old })
}

// resetConnPool drops any pooled UDP conn that dialed a now-closed server so
// each test starts from a fresh dial. The pool is recreated with its New
// function intact (a zero-value sync.Pool{} would drop New and return nil).
func resetConnPool() {
	connPool = sync.Pool{New: func() any {
		conn, err := net.Dial("udp", *startup.Upstream)
		if err != nil {
			return &errConn{err: err}
		}
		return conn
	}}
}

// --- tests ----------------------------------------------------------------

// TestSendUdpQuery proves the resolver sends a query over UDP and returns a
// valid DNS reply from a live upstream.
func TestSendUdpQuery(t *testing.T) {
	upstream, cleanup := startUDPServer(t)
	defer cleanup()
	setUpstream(t, upstream)
	resetConnPool()
	*startup.Ttl = 0

	r := New("udp-query")
	resp, err := r.SendUdpQuery(makeDNSQuery(1, "example.com"))
	if err != nil {
		t.Fatalf("SendUdpQuery: %v", err)
	}
	if len(resp) < 12 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}
	if resp[2]&0x80 == 0 {
		t.Error("expected QR=1 set in the reply")
	}
}

// TestSendTcpQuery proves the resolver sends a length-prefixed query over TCP
// and returns a valid DNS reply from a live upstream.
func TestSendTcpQuery(t *testing.T) {
	upstream, cleanup := startTCPServer(t)
	defer cleanup()
	setUpstream(t, upstream)
	*startup.Ttl = 0

	r := New("tcp-query")
	resp, err := r.SendTcpQuery(makeDNSQuery(7, "example.com"))
	if err != nil {
		t.Fatalf("SendTcpQuery: %v", err)
	}
	if len(resp) < 12 {
		t.Fatalf("response too short: %d bytes", len(resp))
	}
	if resp[2]&0x80 == 0 {
		t.Error("expected QR=1 set in the reply")
	}
}

// TestPooledBufferIntegrity proves the copy-out fix: a response returned from
// SendUdpQuery must not share its backing array with the pooled read buffer,
// otherwise a subsequent query reusing that buffer would corrupt the earlier
// reply.
func TestPooledBufferIntegrity(t *testing.T) {
	upstream, cleanup := startUDPServer(t)
	defer cleanup()
	setUpstream(t, upstream)
	resetConnPool()
	*startup.Ttl = 0

	r := New("integrity")
	q1 := makeDNSQuery(1, "example.com")
	respA, err := r.SendUdpQuery(q1)
	if err != nil {
		t.Fatalf("first query: %v", err)
	}
	wantA := append([]byte(nil), respA...) // snapshot before the buffer is reused

	q2 := makeDNSQuery(2, "example.org")
	if _, err := r.SendUdpQuery(q2); err != nil {
		t.Fatalf("second query: %v", err)
	}

	if !stdbytes.Equal(respA, wantA) {
		t.Fatalf("pooled response corrupted after buffer reuse: got %x, want %x", respA, wantA)
	}
	if respA[1] != 0x01 {
		t.Fatalf("respA header clobbered by second query: byte0=%x", respA[1])
	}
}
