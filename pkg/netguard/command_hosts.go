package netguard

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// CommandTargetsInternalHost reports whether a plugin command names an
// internal address: a loopback, private, link-local, unique-local or
// unspecified IP (IPv4-mapped IPv6 included), or localhost. It looks at the
// host of every URL in the command (grpc://10.0.0.1:50051), at host:port
// tokens and --flag=host:port values, and at bare IP literals.
//
// URL hosts are what a plugin dials, so there the legacy IPv4 spellings that
// inet_aton and many dialers accept are decoded too: decimal (2130706433),
// hex (0x7f000001), octal (0177.0.0.1) and short forms (127.1). Elsewhere
// only canonical IP literals count, so a version ("10.2"), a ratio ("1:2") or
// a path (/opt/plugins/v10.2/run) is not mistaken for an address.
//
// It does no DNS resolution: a hostname that resolves to an internal address
// is not detected here (the plugin security service resolves and checks URL
// hosts when the API validates the command).
func CommandTargetsInternalHost(command string) bool {
	for _, token := range strings.Fields(command) {
		if strings.Contains(token, "://") {
			if u, err := url.Parse(token); err == nil && IsInternalHost(u.Hostname()) {
				return true
			}
			continue
		}
		// --addr=127.0.0.1:50051 names the address after the '='.
		if i := strings.IndexByte(token, '='); i >= 0 {
			token = token[i+1:]
		}
		if host, _, err := net.SplitHostPort(token); err == nil {
			token = host
		}
		if isLocalhost(token) {
			return true
		}
		if addr, err := netip.ParseAddr(strings.Trim(token, "[]")); err == nil && isInternalAddr(addr) {
			return true
		}
	}
	return false
}

// IsInternalHost reports whether a host literal (no DNS lookup) is localhost
// or an internal IP address, including IPv4-mapped IPv6 forms and the legacy
// IPv4 spellings described on CommandTargetsInternalHost.
func IsInternalHost(host string) bool {
	host = strings.Trim(host, "[]")
	if isLocalhost(host) {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return isInternalAddr(addr)
	}
	if addr, ok := parseLegacyIPv4(host); ok {
		return isInternalAddr(addr)
	}
	return false
}

// isLocalhost matches localhost and the reserved *.localhost names (RFC
// 6761), ignoring case and a trailing dot.
func isLocalhost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

// isInternalAddr classifies an address, unwrapping IPv4-mapped IPv6 first.
func isInternalAddr(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if addr.Is4() && addr.As4()[0] == 0 {
		return true // 0.0.0.0/8 "this network" reaches the local host
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast()
}

// parseLegacyIPv4 decodes the inet_aton forms of an IPv4 address: one to four
// dot-separated parts, each decimal, octal (leading 0) or hex (0x), where the
// last part fills the remaining bytes.
func parseLegacyIPv4(s string) (netip.Addr, bool) {
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return netip.Addr{}, false
	}
	vals := make([]uint64, len(parts))
	for i, p := range parts {
		base := 10
		switch {
		case len(p) > 2 && (p[:2] == "0x" || p[:2] == "0X"):
			base, p = 16, p[2:]
		case len(p) > 1 && p[0] == '0':
			base, p = 8, p[1:]
		}
		if p == "" || p[0] == '+' || p[0] == '-' {
			return netip.Addr{}, false
		}
		v, err := strconv.ParseUint(p, base, 32)
		if err != nil {
			return netip.Addr{}, false
		}
		vals[i] = v
	}
	// Every part but the last is one byte; the last fills the rest.
	last := len(vals) - 1
	if vals[last] >= 1<<(8*uint(4-last)) {
		return netip.Addr{}, false
	}
	n := vals[last]
	for i := 0; i < last; i++ {
		if vals[i] > 0xff {
			return netip.Addr{}, false
		}
		n |= vals[i] << (8 * uint(3-i))
	}
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}), true
}
