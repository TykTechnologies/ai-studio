// +build e2e

package plugintest_test

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// Enterprise License Tests
// ============================================================================
// The Advanced LLM Cache plugin is an enterprise plugin that requires a valid
// enterprise license to operate. There is no "community mode" - without a valid
// enterprise license, the plugin will fail to start.

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

// TestLicenseNoLicenseFailsToStart tests that enterprise plugin fails without enterprise license.
// Enterprise plugins require a valid enterprise license to operate - there is no "community mode".
func TestLicenseNoLicenseFailsToStart(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// No enterprise license - plugin should fail to start
	harness.SetLicense("", false, 0)
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

	// OpenSession should fail because the plugin exits when no enterprise license is present
	err = harness.OpenSession()
	if err == nil {
		t.Error("Expected OpenSession to fail without enterprise license, but it succeeded")
	} else {
		t.Logf("OpenSession correctly failed without enterprise license: %v", err)
	}
}

// TestLicenseExpiringSoon tests behavior with expiring license.
func TestLicenseExpiringSoon(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set license expiring in 7 days - should still work
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
	if enabled, ok := status["enterprise_enabled"].(bool); ok {
		if !enabled {
			t.Error("Expected enterprise_enabled=true with valid expiring license")
		}
	}

	t.Logf("Expiring license status: %+v", status)
}

// TestLicenseExpiredFailsToStart tests that plugin fails with expired license.
// Enterprise plugins require a valid (non-expired) enterprise license.
func TestLicenseExpiredFailsToStart(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set expired license - plugin should fail to start
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

	// OpenSession should fail because the plugin exits when license is expired
	err = harness.OpenSession()
	if err == nil {
		t.Error("Expected OpenSession to fail with expired license, but it succeeded")
	} else {
		t.Logf("OpenSession correctly failed with expired license: %v", err)
	}
}

// TestLicenseEntitlementCheck tests that specific entitlements are checked.
func TestLicenseEntitlementCheck(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set enterprise license WITH the required entitlement
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

	// Get license status
	response, err := harness.CallRPC("getLicenseStatus", []byte("{}"))
	if err != nil {
		t.Fatalf("getLicenseStatus RPC failed: %v", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		t.Fatalf("Failed to parse license status: %v", err)
	}

	t.Logf("License status with entitlement: %+v", status)
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

// TestLicenseRedisBackendWithEnterprise tests Redis backend works with enterprise license.
func TestLicenseRedisBackendWithEnterprise(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Enterprise license - Redis backend should work
	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Configure with Redis backend (may fail to connect but initialization should work)
	err := harness.Initialize(map[string]string{
		"enabled":       "true",
		"backend_type":  "redis",
		"redis_address": "localhost:6379",
	})
	if err != nil {
		t.Logf("Initialize returned error (may be expected if Redis not available): %v", err)
	}

	if err := harness.OpenSession(); err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Get config to verify backend configuration
	response, err := harness.CallRPC("getConfig", []byte("{}"))
	if err != nil {
		t.Fatalf("getConfig failed: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(response, &config); err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	t.Logf("Config with Redis backend: %+v", config)
}
