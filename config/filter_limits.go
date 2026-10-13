package config

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// FilterLimit returns the value of the tuning limit called name: Studio's
// configured value (AppConf.FilterLimits) when name is one of
// FilterLimitNames, else the environment. The microgateway has no Studio
// configuration and reads the environment only, and so does every name not
// in FilterLimitNames, such as the security switch FILTER_SCRIPT_ALLOW_OS.
// The value is trimmed; "" means unset.
func FilterLimit(name string) string {
	if conf := Installed(); conf != nil && slices.Contains(FilterLimitNames, name) {
		if v := strings.TrimSpace(conf.FilterLimits[name]); v != "" {
			return v
		}
	}
	return strings.TrimSpace(os.Getenv(name))
}

// FilterLimitInt64 is the limit called name as an integer, or def when it is
// unset or not an integer.
func FilterLimitInt64(name string, def int64) int64 {
	n, err := strconv.ParseInt(FilterLimit(name), 10, 64)
	if err != nil {
		return def
	}
	return n
}

// FilterLimitDuration is the limit called name as a duration, or def when it
// is unset or unparseable. A bare integer is read as seconds, so "10" and
// "10s" mean the same thing in a compose file.
func FilterLimitDuration(name string, def time.Duration) time.Duration {
	v := FilterLimit(name)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}
