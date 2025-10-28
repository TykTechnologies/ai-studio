package main

import (
	"io/fs"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	t.Log("Testing embedded assets in the plugin binary...")

	// List all embedded files
	err := fs.WalkDir(embeddedAssets, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			t.Logf("📁 Directory: %s", path)
		} else {
			info, _ := d.Info()
			t.Logf("📄 File: %s (%d bytes)", path, info.Size())
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk embedded filesystem: %v", err)
	}

	// Test specific files we expect
	expectedFiles := []string{
		"plugin.manifest.json",
		"ui/webc/dashboard.js",
		"ui/webc/settings.js",
		"assets/rate-limit.svg",
	}

	for _, expectedFile := range expectedFiles {
		t.Run("check_"+expectedFile, func(t *testing.T) {
			content, err := embeddedAssets.ReadFile(expectedFile)
			if err != nil {
				t.Errorf("Expected file %s not found: %v", expectedFile, err)
			} else {
				t.Logf("✅ Found %s (%d bytes)", expectedFile, len(content))
			}
		})
	}
}

func TestGetAssetMethod(t *testing.T) {
	// Test asset reading directly from embedded filesystem
	testCases := []string{
		"ui/webc/dashboard.js",
		"ui/webc/settings.js",
		"assets/rate-limit.svg",
		"plugin.manifest.json",
	}

	for _, assetPath := range testCases {
		t.Run("get_asset_"+assetPath, func(t *testing.T) {
			content, err := embeddedAssets.ReadFile(assetPath)
			if err != nil {
				t.Errorf("GetAsset failed for %s: %v", assetPath, err)
			} else {
				t.Logf("✅ GetAsset successful for %s (%d bytes)", assetPath, len(content))
			}
		})
	}
}