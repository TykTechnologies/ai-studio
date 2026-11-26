package config

import "testing"

// Both telemetry channels are off unless the operator turns them on.
func TestTelemetryOffByDefault(t *testing.T) {
	cases := []struct {
		name               string
		env                map[string]string
		wantUsage          bool
		wantLicenseDisable bool
	}{
		{"unset", nil, false, true},
		{"empty", map[string]string{"TELEMETRY_ENABLED": "", "LICENSE_DISABLE_TELEMETRY": ""}, false, true},
		{"opted in", map[string]string{"TELEMETRY_ENABLED": "true", "LICENSE_DISABLE_TELEMETRY": "false"}, true, false},
		{"opted in numerically", map[string]string{"TELEMETRY_ENABLED": "1", "LICENSE_DISABLE_TELEMETRY": "0"}, true, false},
		{"explicitly off", map[string]string{"TELEMETRY_ENABLED": "false", "LICENSE_DISABLE_TELEMETRY": "true"}, false, true},
		{"unrecognized values", map[string]string{"TELEMETRY_ENABLED": "yes", "LICENSE_DISABLE_TELEMETRY": "no"}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conf := LoadFrom(func(k string) string { return tc.env[k] })
			if conf.TelemetryEnabled != tc.wantUsage {
				t.Errorf("TelemetryEnabled = %v, want %v", conf.TelemetryEnabled, tc.wantUsage)
			}
			if conf.LicenseDisableTelemetry != tc.wantLicenseDisable {
				t.Errorf("LicenseDisableTelemetry = %v, want %v", conf.LicenseDisableTelemetry, tc.wantLicenseDisable)
			}
		})
	}
}
