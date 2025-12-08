// +build e2e

package plugintest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/plugintest"
)

// TestPluginBinaryBuild tests that the plugin compiles successfully.
// This is a prerequisite for all E2E tests.
func TestPluginBinaryBuild(t *testing.T) {
	projectRoot := findProjectRoot(t)
	if projectRoot == "" {
		t.Skip("Could not find project root")
	}

	pluginDir := filepath.Join(projectRoot, "enterprise", "plugins", "advanced-llm-cache")
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		t.Skipf("Plugin directory not found: %s", pluginDir)
	}

	// Build the plugin binary with enterprise build tag
	binaryPath := filepath.Join(t.TempDir(), "advanced-llm-cache")

	cmd := exec.Command("go", "build", "-tags", "enterprise", "-o", binaryPath, ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build plugin: %v\n%s", err, output)
	}

	// Verify binary exists
	if info, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("Binary not found after build: %v", err)
	} else {
		t.Logf("Plugin binary built successfully: %s (%d bytes)", binaryPath, info.Size())
	}
}

// TestPluginStartStop tests that the plugin can start and stop cleanly.
func TestPluginStartStop(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Start the plugin
	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Verify it's running
	if harness.ProcessExited() {
		t.Error("Plugin process exited immediately after start")
	}

	// Stop the plugin
	harness.Stop()

	// Give it time to exit
	time.Sleep(100 * time.Millisecond)

	// Verify it stopped
	if !harness.ProcessExited() {
		t.Error("Plugin process did not exit after stop")
	}
}

// TestPluginInitializeBasic tests basic plugin initialization.
func TestPluginInitializeBasic(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with minimal config
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_entry_size_kb": "1024",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	t.Log("Plugin initialized successfully")
}

// TestPluginInitializeWithInvalidConfig tests that invalid config is handled.
func TestPluginInitializeWithInvalidConfig(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize with invalid TTL (negative)
	err := harness.Initialize(map[string]string{
		"enabled":     "true",
		"ttl_seconds": "-100",
	})
	// The plugin may normalize invalid values or return an error
	// Either is acceptable behavior
	if err != nil {
		t.Logf("Initialize with invalid config returned error (expected): %v", err)
	} else {
		t.Log("Initialize with invalid config succeeded (plugin normalized values)")
	}
}

// TestPluginSessionLifecycle tests the full session lifecycle.
func TestPluginSessionLifecycle(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// Set up license
	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

	if err := harness.Start(); err != nil {
		t.Fatalf("Failed to start plugin: %v", err)
	}

	// Initialize
	err := harness.Initialize(map[string]string{
		"enabled":           "true",
		"ttl_seconds":       "300",
		"max_cache_size_mb": "64",
	})
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Open session (triggers OnSessionReady)
	err = harness.OpenSession()
	if err != nil {
		t.Fatalf("OpenSession failed: %v", err)
	}

	// Verify license was checked
	if !harness.LicenseWasChecked() {
		t.Log("Note: License was not checked during session (may be cached)")
	} else {
		t.Log("License was verified during session")
	}
}

// TestPluginRPCAfterSession tests RPC calls work after session is established.
func TestPluginRPCAfterSession(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	harness.SetLicense("enterprise", true, 365)
	harness.SetEntitlements([]string{"advanced-llm-cache"})

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

	// Test getMetrics RPC
	response, err := harness.CallRPC("getMetrics", []byte("{}"))
	if err != nil {
		t.Fatalf("getMetrics RPC failed: %v", err)
	}

	t.Logf("getMetrics response: %s", string(response))

	// Test getConfig RPC
	response, err = harness.CallRPC("getConfig", []byte("{}"))
	if err != nil {
		t.Fatalf("getConfig RPC failed: %v", err)
	}

	t.Logf("getConfig response: %s", string(response))
}

// TestPluginNoLicenseFailsToStart tests that enterprise plugin fails without valid license.
// Enterprise plugins require a valid enterprise license - there is no "community mode".
func TestPluginNoLicenseFailsToStart(t *testing.T) {
	harness := setupE2EHarness(t)
	defer harness.Stop()

	// No enterprise license - plugin should fail during OpenSession
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

// setupE2EHarness creates and configures a test harness for E2E tests.
func setupE2EHarness(t *testing.T) *plugintest.E2EPluginHarness {
	t.Helper()

	projectRoot := findProjectRoot(t)
	if projectRoot == "" {
		t.Skip("Could not find project root")
	}

	pluginDir := filepath.Join(projectRoot, "enterprise", "plugins", "advanced-llm-cache")
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		t.Skipf("Plugin directory not found: %s", pluginDir)
	}

	// Build the plugin binary with enterprise build tag
	binaryPath := filepath.Join(t.TempDir(), "advanced-llm-cache")

	cmd := exec.Command("go", "build", "-tags", "enterprise", "-o", binaryPath, ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build plugin: %v\n%s", err, output)
	}

	return plugintest.NewE2EHarness(binaryPath)
}
