// Package plugintest provides testing utilities for plugin integration tests.
// It enables true E2E testing by spawning real plugin subprocesses via go-plugin.
package plugintest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExtractRPCMethodsFromUI scans JS files for pluginAPI.call('methodName', ...) patterns.
// This is used to validate that all UI RPC calls have corresponding handlers in the plugin.
// Returns a deduplicated, sorted list of method names.
func ExtractRPCMethodsFromUI(uiDir string) ([]string, error) {
	var methods []string
	seen := make(map[string]bool)

	// Match patterns like:
	// pluginAPI.call('methodName', ...)
	// pluginAPI.call("methodName", ...)
	// this.pluginAPI.call('methodName', ...)
	pattern := regexp.MustCompile(`(?:this\.)?pluginAPI\.call\(['"](\w+)['"]`)

	err := filepath.Walk(uiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JS files
		if info.IsDir() || filepath.Ext(path) != ".js" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		matches := pattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 && !seen[match[1]] {
				methods = append(methods, match[1])
				seen[match[1]] = true
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort for consistent output
	sort.Strings(methods)
	return methods, nil
}

// RPCMethodInfo contains information about an RPC method extracted from UI
type RPCMethodInfo struct {
	Method   string // Method name (e.g., "getMetrics")
	File     string // Source file where it was found
	Line     int    // Line number (approximate)
	Context  string // Surrounding code context
}

// ExtractRPCMethodsWithContext scans JS files and returns detailed info about each RPC call.
// This is useful for debugging contract mismatches.
func ExtractRPCMethodsWithContext(uiDir string) ([]RPCMethodInfo, error) {
	var methods []RPCMethodInfo
	seen := make(map[string]bool)

	pattern := regexp.MustCompile(`(?:this\.)?pluginAPI\.call\(['"](\w+)['"]`)

	err := filepath.Walk(uiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || filepath.Ext(path) != ".js" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(content), "\n")
		for lineNum, line := range lines {
			matches := pattern.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 1 {
					methodName := match[1]
					// Use method+file as key to track all occurrences
					key := methodName + ":" + path
					if !seen[key] {
						methods = append(methods, RPCMethodInfo{
							Method:  methodName,
							File:    path,
							Line:    lineNum + 1,
							Context: strings.TrimSpace(line),
						})
						seen[key] = true
					}
				}
			}
		}
		return nil
	})

	return methods, err
}

// ContractViolation represents a mismatch between UI expectations and plugin implementation
type ContractViolation struct {
	Method      string   // Method name that's missing or erroring
	UIFiles     []string // Files where this method is called
	ErrorType   string   // "missing" or "error"
	ErrorDetail string   // Additional error information
}

// RPCHandler is an interface for anything that can handle RPC calls (plugins)
type RPCHandler interface {
	HandleRPC(method string, payload []byte) ([]byte, error)
}

// ValidateRPCContract checks that all UI RPC methods are implemented by the plugin.
// Returns a list of contract violations (methods not implemented or returning errors).
func ValidateRPCContract(handler RPCHandler, uiDir string) ([]ContractViolation, error) {
	methods, err := ExtractRPCMethodsWithContext(uiDir)
	if err != nil {
		return nil, err
	}

	// Group by method name
	methodFiles := make(map[string][]string)
	for _, m := range methods {
		methodFiles[m.Method] = append(methodFiles[m.Method], m.File)
	}

	var violations []ContractViolation

	for method, files := range methodFiles {
		// Try calling the method with empty payload
		_, err := handler.HandleRPC(method, []byte("{}"))
		if err != nil {
			errStr := err.Error()
			violation := ContractViolation{
				Method:      method,
				UIFiles:     files,
				ErrorDetail: errStr,
			}

			// Check if it's a "not implemented" style error
			if strings.Contains(strings.ToLower(errStr), "unknown") ||
				strings.Contains(strings.ToLower(errStr), "not implemented") ||
				strings.Contains(strings.ToLower(errStr), "not found") ||
				strings.Contains(strings.ToLower(errStr), "unsupported") {
				violation.ErrorType = "missing"
			} else {
				violation.ErrorType = "error"
			}

			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// GetExpectedRPCMethods returns a static list of expected RPC methods for advanced-llm-cache.
// This serves as documentation and a quick reference.
func GetExpectedRPCMethods() map[string]string {
	return map[string]string{
		"getMetrics":       "Cache statistics (hits, misses, entry count)",
		"clearCache":       "Clear all cached entries",
		"getConfig":        "Get plugin configuration",
		"getClearStatus":   "Get distributed clear progress",
		"getHealth":        "Backend health check",
		"getBackendHealth": "Alias for getHealth",
		"testBackend":      "Connection test with latency measurement",
		"getLicenseStatus": "License and enterprise feature status",
		"listEntries":      "Browse cache entries with pagination",
		"deleteEntry":      "Delete a single cache entry by key",
	}
}
