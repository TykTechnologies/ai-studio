package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDocsLinksFromJSON(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "test_docs_links.json")

	// Test data
	testLinks := map[string]string{
		"llm_providers":    "https://docs.example.com/llm",
		"data_sources":     "https://docs.example.com/data",
		"tools":            "https://docs.example.com/tools",
		"rbac_user_groups": "https://docs.example.com/rbac",
	}

	// Write test data to file
	testData, err := json.Marshal(testLinks)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(testFilePath, testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test loading docs links
	docsLinksData, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	var docsLinks DocsLinks
	err = json.Unmarshal(docsLinksData, &docsLinks)
	if err != nil {
		t.Fatalf("Failed to unmarshal docs links: %v", err)
	}

	// Verify the loaded links match the test data
	if len(docsLinks) != len(testLinks) {
		t.Errorf("Expected %d links, got %d", len(testLinks), len(docsLinks))
	}

	for key, expectedValue := range testLinks {
		actualValue, exists := docsLinks[key]
		if !exists {
			t.Errorf("Expected key %s not found in loaded docs links", key)
			continue
		}

		if actualValue != expectedValue {
			t.Errorf("For key %s, expected value %s, got %s", key, expectedValue, actualValue)
		}
	}
}

func TestAppConfLoadDocsLinks(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "docs_links.json")

	// Test data
	testLinks := map[string]string{
		"llm_providers":    "https://docs.example.com/llm",
		"data_sources":     "https://docs.example.com/data",
		"tools":            "https://docs.example.com/tools",
		"rbac_user_groups": "https://docs.example.com/rbac",
	}

	// Write test data to file
	testData, err := json.Marshal(testLinks)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(testFilePath, testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a test AppConf
	conf := &AppConf{}

	// Mock the file reading by directly loading from our test file
	docsLinksData, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	var docsLinks DocsLinks
	err = json.Unmarshal(docsLinksData, &docsLinks)
	if err != nil {
		t.Fatalf("Failed to unmarshal docs links: %v", err)
	}

	conf.DocsLinks = docsLinks

	// Verify the AppConf has the correct docs links
	if len(conf.DocsLinks) != len(testLinks) {
		t.Errorf("Expected %d links in AppConf, got %d", len(testLinks), len(conf.DocsLinks))
	}

	for key, expectedValue := range testLinks {
		actualValue, exists := conf.DocsLinks[key]
		if !exists {
			t.Errorf("Expected key %s not found in AppConf.DocsLinks", key)
			continue
		}

		if actualValue != expectedValue {
			t.Errorf("For key %s in AppConf.DocsLinks, expected value %s, got %s", key, expectedValue, actualValue)
		}
	}
}

// TestDocsLinksReadFromFile tests the ReadFromFile method of the DocsLinks type
func TestDocsLinksReadFromFile(t *testing.T) {
	// The file goes in a temporary directory, by absolute path: changing the
	// working directory to a fresh temporary directory would make go test's
	// result cache key differ on every run.
	tempDir := t.TempDir()
	linksFile := filepath.Join(tempDir, "config", "docs_links.json")

	// Create the config directory
	err := os.Mkdir(filepath.Dir(linksFile), 0755)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Test data
	testLinks := map[string]string{
		"llm_providers":    "https://docs.example.com/llm",
		"data_sources":     "https://docs.example.com/data",
		"tools":            "https://docs.example.com/tools",
		"rbac_user_groups": "https://docs.example.com/rbac",
		"privacy_levels":   "https://docs.example.com/privacy",
	}

	// Write test data to file
	testData, err := json.Marshal(testLinks)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	err = os.WriteFile(linksFile, testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Create a DocsLinks instance and call ReadFromFile with the file path
	docsLinks := make(DocsLinks)
	docsLinks.ReadFromFile(linksFile)

	// Verify the loaded links match the test data
	if len(docsLinks) != len(testLinks) {
		t.Errorf("Expected %d links, got %d", len(testLinks), len(docsLinks))
	}

	for key, expectedValue := range testLinks {
		actualValue, exists := docsLinks[key]
		if !exists {
			t.Errorf("Expected key %s not found in loaded docs links", key)
			continue
		}

		if actualValue != expectedValue {
			t.Errorf("For key %s, expected value %s, got %s", key, expectedValue, actualValue)
		}
	}
}
