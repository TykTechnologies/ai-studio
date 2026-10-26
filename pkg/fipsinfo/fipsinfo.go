// Package fipsinfo reports the process's FIPS 140-3 status, for the startup
// log, the /health endpoints of AI Studio and the microgateway, and the edge
// heartbeat (so the hub can show each edge's status).
//
// A FIPS build (GOFIPS140=v1.0.0) defaults to fips140=on, but an operator can
// still turn FIPS mode off at run time with GODEBUG=fips140=off; Enabled then
// reports false, which is what this package makes visible.
package fipsinfo

import (
	"crypto/fips140"
	"strconv"
)

// Status is the FIPS 140-3 status of a process.
type Status struct {
	// Enabled: FIPS 140-3 mode is on (crypto/fips140.Enabled).
	Enabled bool `json:"enabled"`
	// Version is the Go Cryptographic Module version in use, such as
	// "v1.0.0-c2097c7c" for a GOFIPS140=v1.0.0 build (crypto/fips140.Version).
	Version string `json:"version"`
	// Enforced: non-approved algorithms are refused (GODEBUG=fips140=only).
	Enforced bool `json:"enforced"`
}

// Current returns this process's status.
func Current() Status {
	return Status{Enabled: fips140.Enabled(), Version: fips140.Version(), Enforced: fips140.Enforced()}
}

// Keys of the status in the edge heartbeat's health metrics
// (HeartbeatRequest.health.metrics). Edges released before this field send
// none, and the hub then shows the status as unknown.
const (
	MetricEnabled  = "fips.enabled"
	MetricVersion  = "fips.version"
	MetricEnforced = "fips.enforced"
)

// Metrics returns the status as heartbeat health metrics.
func (s Status) Metrics() map[string]string {
	return map[string]string{
		MetricEnabled:  strconv.FormatBool(s.Enabled),
		MetricVersion:  s.Version,
		MetricEnforced: strconv.FormatBool(s.Enforced),
	}
}

// FromMetrics reads a status from heartbeat health metrics; ok is false when
// the edge did not report one (or reported something unreadable).
func FromMetrics(m map[string]string) (s Status, ok bool) {
	enabled, err := strconv.ParseBool(m[MetricEnabled])
	if err != nil {
		return Status{}, false
	}
	enforced, err := strconv.ParseBool(m[MetricEnforced])
	if err != nil {
		return Status{}, false
	}
	return Status{Enabled: enabled, Version: m[MetricVersion], Enforced: enforced}, true
}
