package proxy

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

// internalHopToken is the value InternalRoutingTransport puts in
// hdrInternalHop on the gateway's own /ai/ -> /llm/call/ loopback hop. It is
// random per process, so a client cannot claim to be the gateway by sending
// the header, and neither can a reverse proxy on the same host that forwards
// it (to the gateway such a request also arrives over loopback).
var internalHopToken = newInternalHopToken()

func newInternalHopToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("proxy: reading random bytes for the internal hop token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// hdrStreamBlocked is the trailer with which the inner hop of the gateway's
// own loopback says a response filter blocked the stream it answered, with
// the block message. The outer hop (the /ai/ bridge and what routes through
// it) reads it whatever its SDK driver made of the error event in the
// stream, so a block always reaches the client, and the tool calls the
// driver assembled are never emitted.
const hdrStreamBlocked = "X-Tyk-Stream-Blocked"

// markStreamBlocked sets the block trailer on a stream answered to the
// gateway's own loopback hop. Other clients get the vendor's error event
// only.
func markStreamBlocked(w http.ResponseWriter, r *http.Request, msg string) {
	if r == nil || !IsInternalHop(r) {
		return
	}
	msg = strings.Join(strings.Fields(msg), " ")
	if msg == "" {
		msg = "blocked by policy"
	}
	w.Header().Set(http.TrailerPrefix+hdrStreamBlocked, msg)
}

// IsInternalHop reports whether r is the gateway calling itself on its
// loopback hop: it carries this process's hop token and arrives over a
// loopback connection.
func IsInternalHop(r *http.Request) bool {
	v := r.Header.Get(hdrInternalHop)
	if v == "" || subtle.ConstantTimeCompare([]byte(v), []byte(internalHopToken)) != 1 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
