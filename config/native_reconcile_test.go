package config

import (
	"testing"
	"time"
)

// The reconciler timings load as Go durations and counts; unset, invalid or
// negative values stay 0, which keeps the gateway's defaults.
func TestLoadFrom_NativeReconcilerTimings(t *testing.T) {
	vals := map[string]string{
		"NATIVE_API_RECONCILE_INTERVAL": "45s",
		"NATIVE_API_RECONCILE_BATCH":    "200",
		"NATIVE_API_RECONCILE_WORKERS":  "8",
		"NATIVE_API_POLL_INTERVAL":      "5m",
		"NATIVE_API_POLL_MAX_BACKOFF":   "2h",
	}
	conf := LoadFrom(func(k string) string { return vals[k] })
	if conf.NativeReconcileInterval != 45*time.Second || conf.NativeReconcileBatch != 200 ||
		conf.NativeReconcileWorkers != 8 || conf.NativePollInterval != 5*time.Minute ||
		conf.NativePollMaxBackoff != 2*time.Hour {
		t.Errorf("reconciler timings not applied: %v %d %d %v %v", conf.NativeReconcileInterval, conf.NativeReconcileBatch,
			conf.NativeReconcileWorkers, conf.NativePollInterval, conf.NativePollMaxBackoff)
	}

	for _, bad := range []map[string]string{
		{},
		{
			"NATIVE_API_RECONCILE_INTERVAL": "30", // no unit
			"NATIVE_API_RECONCILE_BATCH":    "-5",
			"NATIVE_API_RECONCILE_WORKERS":  "four",
			"NATIVE_API_POLL_INTERVAL":      "-1m",
			"NATIVE_API_POLL_MAX_BACKOFF":   "soon",
		},
		{
			"NATIVE_API_RECONCILE_INTERVAL": "0s",
			"NATIVE_API_RECONCILE_BATCH":    "0",
			"NATIVE_API_RECONCILE_WORKERS":  "0",
			"NATIVE_API_POLL_INTERVAL":      "0",
			"NATIVE_API_POLL_MAX_BACKOFF":   " ",
		},
	} {
		conf := LoadFrom(func(k string) string { return bad[k] })
		if conf.NativeReconcileInterval != 0 || conf.NativeReconcileBatch != 0 || conf.NativeReconcileWorkers != 0 ||
			conf.NativePollInterval != 0 || conf.NativePollMaxBackoff != 0 {
			t.Errorf("%v: want all 0 (defaults), got %v %d %d %v %v", bad, conf.NativeReconcileInterval, conf.NativeReconcileBatch,
				conf.NativeReconcileWorkers, conf.NativePollInterval, conf.NativePollMaxBackoff)
		}
	}
}
