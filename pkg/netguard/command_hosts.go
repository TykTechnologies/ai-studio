package netguard

import (
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// CommandTargetsInternalHost reports whether a plugin command names an
// internal address: a loopback, private, link-local, unique-local, shared
// (CGNAT, 100.64.0.0/10) or unspecified IP (IPv4-mapped IPv6 included), or
// localhost. It looks at the host of every URL in the command
// (grpc://10.0.0.1:50051), at host:port tokens, and at bare IP literals.
// Flag values (--upstream=grpc://10.0.0.1), quoted values and comma-separated
// lists are unpacked first.
//
// URL hosts are what a plugin dials, so there the legacy IPv4 spellings that
// inet_aton and many dialers accept are decoded too: decimal (2130706433),
// hex (0x7f000001), octal (0177.0.0.1) and short forms (127.1). Elsewhere
// only canonical IP literals count, so a version ("10.2"), a ratio ("1:2") or
// a path (/opt/plugins/v10.2/run) is not mistaken for an address.
//
// It does no DNS resolution: a hostname that resolves to an internal address
// is not detected here (the plugin security service resolves and checks URL
// hosts when the API validates the command). Translated IPv6 forms (NAT64,
// IPv4-compatible, 6to4) are not decoded.
func CommandTargetsInternalHost(command string) bool {
	for _, token := range strings.Fields(command) {
		for _, value := range commandValues(token) {
			if valueTargetsInternalHost(value) {
				return true
			}
		}
	}
	return false
}

// commandValues unpacks a command token into the values that may name a
// host: the part after a flag's '=' (unless the '=' belongs to a URL's
// query), split on commas, with surrounding quotes removed.
func commandValues(token string) []string {
	token = unquote(token)
	if eq := strings.IndexByte(token, '='); eq >= 0 {
		if scheme := strings.Index(token, "://"); scheme < 0 || eq < scheme {
			token = unquote(token[eq+1:])
		}
	}
	values := strings.Split(token, ",")
	for i, v := range values {
		values[i] = unquote(v)
	}
	return values
}

func unquote(s string) string {
	return strings.Trim(s, `"'`)
}

func valueTargetsInternalHost(value string) bool {
	if i := strings.Index(value, "://"); i >= 0 {
		// Check both what url.Parse sees and the authority right after the
		// "://": a value such as upstream:grpc://10.0.0.1 parses as an opaque
		// URL with no host, and a malformed one does not parse at all.
		if u, err := url.Parse(value); err == nil && IsInternalHost(u.Hostname()) {
			return true
		}
		return IsInternalHost(authorityHost(value[i+3:]))
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	if isLocalhost(value) {
		return true
	}
	addr, err := netip.ParseAddr(strings.Trim(value, "[]"))
	return err == nil && isInternalAddr(addr)
}

// authorityHost reads the host from the text after a URL's "://": up to the
// first / ? or #, without userinfo, port or wrapping punctuation such as the
// ")" of "(http://10.0.0.1)".
func authorityHost(afterScheme string) string {
	authority := afterScheme
	if i := strings.IndexAny(authority, "/?#"); i >= 0 {
		authority = authority[:i]
	}
	if i := strings.LastIndexByte(authority, '@'); i >= 0 {
		authority = authority[i+1:]
	}
	authority = strings.TrimRight(authority, ")>")
	if host, _, err := net.SplitHostPort(authority); err == nil {
		return host
	}
	return authority
}

// IsInternalHost reports whether a host literal (no DNS lookup) is localhost
// or an internal IP address, including IPv4-mapped IPv6 forms, a trailing
// dot, and the legacy IPv4 spellings described on CommandTargetsInternalHost.
func IsInternalHost(host string) bool {
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	if isLocalhost(host) {
		return true
	}
	if i := strings.Index(host, "%25"); i >= 0 {
		host = host[:i] // URL-escaped IPv6 zone
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

// sharedAddressSpace is 100.64.0.0/10 (RFC 6598).
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// isInternalAddr classifies an address, unwrapping IPv4-mapped IPv6 first.
func isInternalAddr(addr netip.Addr) bool {
	addr = addr.WithZone("").Unmap()
	if addr.Is4() && addr.As4()[0] == 0 {
		return true // 0.0.0.0/8 "this network" reaches the local host
	}
	if sharedAddressSpace.Contains(addr) {
		return true // CGNAT; also some cloud metadata endpoints (100.100.100.200)
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
