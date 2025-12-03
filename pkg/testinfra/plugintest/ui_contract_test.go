package plugintest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/plugintest"
)

// TestAdvancedLLMCacheUIContract validates that UI RPC calls match plugin handlers.
// This test scans the plugin's UI JavaScript files and verifies all called methods
// are implemented in the plugin.
func TestAdvancedLLMCacheUIContract(t *testing.T) {
	// Find the project root by looking for go.mod
	projectRoot := findProjectRoot(t)
	if projectRoot == "" {
		t.Skip("Could not find project root (go.mod)")
	}

	uiDir := filepath.Join(projectRoot, "enterprise", "plugins", "advanced-llm-cache", "ui", "webc")

	// Check if the UI directory exists
	if _, err := os.Stat(uiDir); os.IsNotExist(err) {
		t.Skipf("UI directory not found: %s", uiDir)
	}

	// Extract RPC methods from the UI JavaScript files
	uiMethods, err := plugintest.ExtractRPCMethodsFromUI(uiDir)
	if err != nil {
		t.Fatalf("Failed to extract RPC methods: %v", err)
	}

	t.Logf("Found %d unique RPC methods in UI files:", len(uiMethods))
	for _, method := range uiMethods {
		t.Logf("  - %s", method)
	}

	// Known RPC methods implemented by the advanced-llm-cache plugin
	implementedMethods := map[string]bool{
		"getMetrics":       true,
		"clearCache":       true,
		"getConfig":        true,
		"getClearStatus":   true,
		"getBackendHealth": true,
		"getHealth":        true,
		"testBackend":      true,
		"getLicenseStatus": true,
		"listEntries":      true,
		"deleteEntry":      true,
	}

	// Check for missing methods
	var missingMethods []string
	for _, uiMethod := range uiMethods {
		if !implementedMethods[uiMethod] {
			missingMethods = append(missingMethods, uiMethod)
		}
	}

	if len(missingMethods) > 0 {
		t.Errorf("UI calls %d RPC method(s) not implemented in the plugin:", len(missingMethods))
		for _, missing := range missingMethods {
			t.Errorf("  - %s", missing)
		}
	}

	// Check for unused methods (informational)
	uiMethodSet := make(map[string]bool)
	for _, m := range uiMethods {
		uiMethodSet[m] = true
	}

	var unusedMethods []string
	for method := range implementedMethods {
		if !uiMethodSet[method] {
			unusedMethods = append(unusedMethods, method)
		}
	}

	if len(unusedMethods) > 0 {
		t.Logf("Note: %d method(s) implemented but not called by UI:", len(unusedMethods))
		for _, unused := range unusedMethods {
			t.Logf("  - %s", unused)
		}
	}

	// Report coverage
	covered := len(uiMethods) - len(missingMethods)
	coverage := float64(covered) / float64(len(uiMethods)) * 100
	t.Logf("UI coverage: %.1f%% (%d/%d UI methods are implemented)", coverage, covered, len(uiMethods))
}

// TestUIRPCMethodExtraction validates the RPC extractor works correctly.
func TestUIRPCMethodExtraction(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()

	// Create a test JS file with various pluginAPI.call patterns
	testJS := `
// Test file with various patterns
class TestComponent extends HTMLElement {
  async loadData() {
    // Standard call
    await this.pluginAPI.call('getMetrics', {});

    // Call with complex payload
    const result = await this.pluginAPI.call('clearCache', { force: true });

    // Double-quoted method name
    await this.pluginAPI.call("getConfig", {});

    // In arrow function
    const fn = async () => {
      return this.pluginAPI.call('testBackend', {});
    };
  }
}
`
	testFile := filepath.Join(tempDir, "test.js")
	if err := os.WriteFile(testFile, []byte(testJS), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Extract methods
	methods, err := plugintest.ExtractRPCMethodsFromUI(tempDir)
	if err != nil {
		t.Fatalf("Failed to extract methods: %v", err)
	}

	// Verify expected methods
	expectedMethods := []string{"clearCache", "getConfig", "getMetrics", "testBackend"}

	if len(methods) != len(expectedMethods) {
		t.Errorf("Expected %d methods, got %d: %v", len(expectedMethods), len(methods), methods)
	}

	for _, expected := range expectedMethods {
		found := false
		for _, method := range methods {
			if method == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected method '%s' not found in extracted methods: %v", expected, methods)
		}
	}

	t.Logf("Successfully extracted %d methods: %v", len(methods), methods)
}

// TestUIRPCMethodsWithContext tests the detailed extraction with file/line info.
func TestUIRPCMethodsWithContext(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file
	testJS := `class Test {
  async load() {
    await this.pluginAPI.call('method1', {});
  }
  async save() {
    await this.pluginAPI.call('method2', {});
  }
}
`
	testFile := filepath.Join(tempDir, "component.js")
	if err := os.WriteFile(testFile, []byte(testJS), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Extract with context
	methods, err := plugintest.ExtractRPCMethodsWithContext(tempDir)
	if err != nil {
		t.Fatalf("Failed to extract methods with context: %v", err)
	}

	if len(methods) != 2 {
		t.Errorf("Expected 2 methods, got %d", len(methods))
	}

	for _, m := range methods {
		t.Logf("Found %s at %s:%d - %s", m.Method, filepath.Base(m.File), m.Line, m.Context)
	}

	// Verify line numbers make sense
	for _, m := range methods {
		if m.Line < 1 {
			t.Errorf("Invalid line number %d for method %s", m.Line, m.Method)
		}
	}
}

// TestExpectedRPCMethods tests the expected methods documentation.
func TestExpectedRPCMethods(t *testing.T) {
	expected := plugintest.GetExpectedRPCMethods()

	if len(expected) == 0 {
		t.Error("GetExpectedRPCMethods returned empty map")
	}

	t.Logf("Expected RPC methods for advanced-llm-cache plugin:")
	for method, desc := range expected {
		t.Logf("  - %s: %s", method, desc)
	}

	// Verify core methods are documented
	coreMethods := []string{"getMetrics", "clearCache", "getConfig", "getHealth"}
	for _, method := range coreMethods {
		if _, ok := expected[method]; !ok {
			t.Errorf("Core method '%s' not documented in GetExpectedRPCMethods", method)
		}
	}
}

// findProjectRoot walks up the directory tree to find go.mod
func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
