package proxy

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
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
