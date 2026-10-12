package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoadFrom_NativeAPILimits(t *testing.T) {
	vals := map[string]string{
		"NATIVE_API_MAX_BODY_BYTES":          "134217728",
		"NATIVE_API_MAX_CONCURRENT_CREATES":  "8",
		"NATIVE_API_MAX_FILTERED_ITEMS":      "500",
		"NATIVE_API_MAX_IN_PROGRESS_PER_APP": "3",
	}
	conf := LoadFrom(func(k string) string { return vals[k] })
	if conf.NativeMaxBodyBytes != 134217728 || conf.NativeMaxConcurrentCreates != 8 ||
		conf.NativeMaxFilteredItems != 500 || conf.NativeMaxInProgressPerApp != 3 {
		t.Errorf("native limits not applied: %d %d %d %d", conf.NativeMaxBodyBytes, conf.NativeMaxConcurrentCreates,
			conf.NativeMaxFilteredItems, conf.NativeMaxInProgressPerApp)
	}

	// Unset, invalid or negative values keep the gateway's defaults (0).
	bad := map[string]string{
		"NATIVE_API_MAX_BODY_BYTES":         "64MiB",
		"NATIVE_API_MAX_CONCURRENT_CREATES": "-1",
	}
	conf = LoadFrom(func(k string) string { return bad[k] })
	if conf.NativeMaxBodyBytes != 0 || conf.NativeMaxConcurrentCreates != 0 ||
		conf.NativeMaxFilteredItems != 0 || conf.NativeMaxInProgressPerApp != 0 {
		t.Errorf("invalid or unset native limits should be 0: %d %d %d %d", conf.NativeMaxBodyBytes,
			conf.NativeMaxConcurrentCreates, conf.NativeMaxFilteredItems, conf.NativeMaxInProgressPerApp)
	}
}

// NATIVE_API_MAX_BODY_BYTES is capped at 256 MiB at load, with a warning;
// values at or below the cap, and unset, are kept.
func TestLoadFrom_NativeMaxBodyBytesCapped(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		want        int64
		warned      bool
	}{
		{"over the cap", "1073741824", NativeMaxBodyBytesLimit, true},
		{"just over the cap", "268435457", NativeMaxBodyBytesLimit, true},
		{"at the cap", "268435456", NativeMaxBodyBytesLimit, false},
		{"below the cap", "1048576", 1048576, false},
		{"unset", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			saved := cfgLog
			cfgLog = zerolog.New(&logs)
			defer func() { cfgLog = saved }()

			conf := LoadFrom(func(k string) string {
				if k == "NATIVE_API_MAX_BODY_BYTES" {
					return tc.value
				}
				return ""
			})
			if conf.NativeMaxBodyBytes != tc.want {
				t.Errorf("NATIVE_API_MAX_BODY_BYTES=%q: got %d, want %d", tc.value, conf.NativeMaxBodyBytes, tc.want)
			}
			wantWarnings := 0
			if tc.warned {
				wantWarnings = 1
			}
			if n := strings.Count(logs.String(), "NATIVE_API_MAX_BODY_BYTES"); n != wantWarnings {
				t.Errorf("NATIVE_API_MAX_BODY_BYTES=%q: %d warnings naming the variable, want %d:\n%s", tc.value, n, wantWarnings, logs.String())
			}
		})
	}
}

func TestClampNativeMaxBodyBytes(t *testing.T) {
	for _, tc := range []struct {
		in, want int64
		warned   bool
	}{
		{0, 0, false},
		{1 << 20, 1 << 20, false},
		{NativeMaxBodyBytesLimit, NativeMaxBodyBytesLimit, false},
		{NativeMaxBodyBytesLimit + 1, NativeMaxBodyBytesLimit, true},
		{1 << 40, NativeMaxBodyBytesLimit, true},
	} {
		var warnedWith int64 = -1
		got := ClampNativeMaxBodyBytes(tc.in, func(n int64) { warnedWith = n })
		if got != tc.want {
			t.Errorf("ClampNativeMaxBodyBytes(%d) = %d, want %d", tc.in, got, tc.want)
		}
		if tc.warned != (warnedWith >= 0) {
			t.Errorf("ClampNativeMaxBodyBytes(%d): warned = %v, want %v", tc.in, warnedWith >= 0, tc.warned)
		}
		if tc.warned && warnedWith != tc.in {
			t.Errorf("ClampNativeMaxBodyBytes(%d) warned with %d, want the configured value", tc.in, warnedWith)
		}
	}
	if got := ClampNativeMaxBodyBytes(1<<40, nil); got != NativeMaxBodyBytesLimit {
		t.Errorf("a nil warn still clamps: got %d", got)
	}
}
