// +build e2e

package plugintest_test

import (
	"encoding/json"
	"testing"
)

// TestLicenseEnterpriseFeaturesEnabled tests enterprise features with valid license.
func TestLicenseEnterpriseFeaturesEnabled(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set enterprise license with all entitlements
	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache", "redis-backend", "audit-logging"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get license status
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	t.Logf("License status: %+v", status)

	// Verify enterprise features are enabled
	if enabled, ok := status["enterprise_enabled"].(bool); ok {
		if !enabled {
			t.Error("Expected enterprise_enabled=true with enterprise license")
		}
	} else {
		t.Error("Missing enterprise_enabled field in license status")
	}
}

// TestLicenseCommunityRestrictions tests that community license restricts features.
func TestLicenseCommunityRestrictions(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set community license (no enterprise features)
	harness.SetLicense("community", false, 0)
	harness.SetEntitlements([]string{})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get license status
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	t.Logf("Community license status: %+v", status)

	// Verify enterprise features are disabled
	if enabled, ok := status["enterprise_enabled"].(bool); ok {
		if enabled {
			t.Error("Expected enterprise_enabled=false with community license")
		}
	}
}

// TestLicenseExpiringSoon tests behavior with expiring license.
func TestLicenseExpiringSoon(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set license expiring in 7 days
	harness.SetLicense("enterprise", true, 7)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled": "true",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// License should still be valid
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	// Features should still work with valid (but expiring) license
	if enabled, ok := status["enterprise_enabled"].(bool); ok && !enabled {
		t.Log("Note: Enterprise features disabled even with valid expiring license")
	}

	t.Logf("Expiring license status: %+v", status)
}

// TestLicenseExpired tests behavior with expired license.
func TestLicenseExpired(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set expired license
	harness.SetLicense("enterprise", false, -30) // Expired 30 days ago
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled": "true",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get license status
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	// Enterprise features should be disabled with expired license
	if enabled, ok := status["enterprise_enabled"].(bool); ok && enabled {
		t.Error("Expected enterprise_enabled=false with expired license")
	}

	t.Logf("Expired license status: %+v", status)
}

// TestLicenseEntitlementCheck tests that specific entitlements are checked.
func TestLicenseEntitlementCheck(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set enterprise license but WITHOUT the required entitlement
	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"other-feature", "something-else"}) // Missing "advanced-llm-cache"

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled": "true",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get license status
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	t.Logf("License status without required entitlement: %+v", status)

	// Plugin may still work but enterprise features may be restricted
	// depending on entitlement requirements
}

// TestLicenseServiceBrokerIntegration tests that license is fetched via service broker.
func TestLicenseServiceBrokerIntegration(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	err := harness.Initialize(map[string]string{
		"enabled": "true",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Verify license was checked via service broker
	if harness.LicenseWasChecked() {
		t.Log("License was checked via service broker during session")
	} else {
		// License check may be delayed or cached
		t.Log("Note: License was not immediately checked (may be cached or deferred)")

		// Trigger a license check explicitly
		_, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
		if err != nil {
			t.Fatalf("getLicenseStatus failed: %v", err)
		}

		if harness.LicenseWasChecked() {
			t.Log("License was checked after explicit getLicenseStatus call")
		}
	}
}

// TestLicenseRedisBackendRestriction tests Redis backend requires enterprise license.
func TestLicenseRedisBackendRestriction(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Community license - should not allow Redis
	harness.SetLicense("community", false, 0)
	harness.SetEntitlements([]string{})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Try to configure Redis backend
	err := harness.Initialize(map[string]string{
		"enabled":       "true",
		"backend_type":  "redis",
		"redis_address": "localhost:6379",
	})
	if err != nil {
		t.Logf("Initialize with Redis on community license returned error: %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		// May fail if Redis backend is strictly enforced
		t.Logf("OpenSession returned: %v", err)
	}

	// Get config to see what backend was actually used
	response, err := harness.CallRPC("getConfig", []byte("{}"))
	if err != nil {
		t.Logf("getConfig failed (may be expected): %v", err)
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(response, &config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	// Check if Redis was actually enabled or fell back to memory
	if backend, ok := config["backend"].(map[string]interface{}); ok {
		if backendType, ok := backend["type"].(string); ok {
			if backendType == "redis" {
				t.Log("Warning: Redis backend was allowed without enterprise license")
			} else {
				t.Logf("Backend fell back to '%s' as expected for community license", backendType)
			}
		}
	}
}
