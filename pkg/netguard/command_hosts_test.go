package netguard

import "testing"

func TestCommandTargetsInternalHost(t *testing.T) {
	for _, tc := range []struct {
		command  string
		internal bool
	}{
		// Canonical forms.
		{"grpc://127.0.0.1:50051", true},
		{"grpc://10.1.2.3:50051", true},
		{"grpc://172.16.0.5:50051", true},
		{"grpc://172.31.255.255:50051", true},
		{"grpc://192.168.1.10:50051", true},
		{"grpc://169.254.169.254:80", true}, // cloud metadata
		{"grpc://0.0.0.0:50051", true},
		{"grpc://0.1.2.3:50051", true},
		{"grpc://localhost:50051", true},
		{"grpc://LOCALHOST:50051", true},
		{"grpc://localhost.:50051", true},
		{"grpc://plugin.localhost:50051", true},

		// Legacy IPv4 spellings in a URL host.
		{"grpc://2130706433:50051", true},      // decimal 127.0.0.1
		{"grpc://0x7f000001:50051", true},      // hex
		{"grpc://0X7F000001:50051", true},      // hex, upper case
		{"grpc://0177.0.0.1:50051", true},      // octal first octet
		{"grpc://0x7f.0.0.1:50051", true},      // hex first octet
		{"grpc://127.1:50051", true},           // short form
		{"grpc://10.1:50051", true},            // short form, private
		{"grpc://192.168.257:50051", true},     // three parts, last fills 16 bits
		{"grpc://127.000.000.001:50051", true}, // zero-padded octets
		{"grpc://3232235777:50051", true},      // decimal 192.168.1.1
		{"grpc://0xa9fea9fe:80", true},         // hex 169.254.169.254
		{"grpc://017700000001:50051", true},    // octal 127.0.0.1

		// IPv6.
		{"grpc://[::1]:50051", true},
		{"grpc://[::]:50051", true},
		{"grpc://[fe80::1]:50051", true},
		{"grpc://[fe80::1%25eth0]:50051", true},
		{"grpc://[fd00::1]:50051", true},
		{"grpc://[fc00::1]:50051", true},
		{"grpc://[::ffff:127.0.0.1]:50051", true},
		{"grpc://[::ffff:7f00:1]:50051", true},
		{"grpc://[::ffff:10.0.0.1]:50051", true},
		{"grpc://[0:0:0:0:0:ffff:192.168.0.1]:50051", true},

		// Outside a URL: host:port, flag values and bare literals.
		{"/usr/local/bin/plugin --addr=127.0.0.1:9000", true},
		{"/usr/local/bin/plugin --upstream 10.0.0.8:443", true},
		{"/usr/local/bin/plugin --host [::1]:9000", true},
		{"/usr/local/bin/plugin --host localhost:9000", true},
		{"/usr/local/bin/plugin 192.168.0.1", true},

		// URLs inside flag values, quotes and lists; malformed URLs.
		{"/usr/local/bin/plugin --target=grpc://127.0.0.1:50051", true},
		{"/usr/local/bin/plugin -u=http://10.0.0.1", true},
		{"/usr/local/bin/plugin --upstream=http://169.254.169.254/", true},
		{"/usr/local/bin/plugin upstream=grpc://[::1]:50051", true},
		{`"grpc://127.0.0.1:50051"`, true},
		{`'grpc://127.0.0.1:50051'`, true},
		{`/usr/local/bin/plugin --addr="127.0.0.1:9000"`, true},
		{`/usr/local/bin/plugin --target="grpc://10.0.0.1:50051"`, true},
		{"/usr/local/bin/plugin --upstreams=grpc://8.8.8.8:1,grpc://10.0.0.1:1", true},
		{"grpc://127.0.0.1:50051/%zz", true},                            // bad escape: url.Parse fails
		{"grpc://user@0x7f000001:50051/%zz", true},                      // userinfo, legacy form, bad escape
		{"/usr/local/bin/plugin upstream:grpc://127.0.0.1:50051", true}, // opaque URL, empty Hostname
		{"/usr/local/bin/plugin x:http://10.0.0.1/", true},
		{"/usr/local/bin/plugin (http://10.0.0.1)", true},
		{"/usr/local/bin/plugin <http://10.0.0.1>", true},
		{"/usr/local/bin/plugin (http://[::1])", true},
		{"grpc://127.0.0.1.:50051", true},                          // trailing dot
		{"grpc://100.100.100.200:80", true},                        // CGNAT range, cloud metadata
		{"grpc://plugins.example.com:50051/?next=10.0.0.1", false}, // '=' in a URL query
		{"grpc://100.128.0.1:50051", false},                        // just above 100.64.0.0/10

		// Public and non-address values.
		{"grpc://8.8.8.8:50051", false},
		{"grpc://172.32.0.1:50051", false},
		{"grpc://[2001:4860:4860::8888]:50051", false},
		{"grpc://plugins.example.com:50051", false},
		{"grpc://my10.example.com:50051", false},
		{"grpc://localhost.example.com:50051", false},
		{"grpc://134744072:50051", false}, // decimal 8.8.8.8
		{"oci://registry.example.com/plugins/foo:v10.2", false},
		{"oci://registry.example.com:5000/plugins/foo:1.0.0", false},
		{"/opt/plugins/v10.2/plugin", false},
		{"/opt/plugins/127.0/plugin", false},
		{"/usr/local/bin/plugin --version 10.2", false},
		{"/usr/local/bin/plugin --ratio=1:2", false},
		{"/usr/local/bin/plugin --start 12:30", false},
		{"file:///opt/plugins/plugin-10.0.0.1", false},
		{"./plugin", false},
		{"", false},
	} {
		if got := CommandTargetsInternalHost(tc.command); got != tc.internal {
			t.Errorf("CommandTargetsInternalHost(%q) = %v, want %v", tc.command, got, tc.internal)
		}
	}
}

func TestParseLegacyIPv4_Rejects(t *testing.T) {
	for _, s := range []string{
		"", ".", "1.2.3.4.5", "256.1.1.1", "1.2.3.256", "1.2.65536", "1.16777216",
		"4294967296", "0x", "0x1g", "08.1.1.1", "1..1", "+1.1.1.1", "-1", "a.b.c.d",
	} {
		if addr, ok := parseLegacyIPv4(s); ok {
			t.Errorf("parseLegacyIPv4(%q) = %v, want rejection", s, addr)
		}
	}
}
