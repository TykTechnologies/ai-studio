package config

import "testing"

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
