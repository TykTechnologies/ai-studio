package templates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFallsBackToEmbeddedDefaults(t *testing.T) {
	t.Chdir(t.TempDir())

	b, err := Read("reset.tmpl")
	if err != nil {
		t.Fatalf("embedded template not found: %v", err)
	}
	want, _ := FS.ReadFile("reset.tmpl")
	if string(b) != string(want) {
		t.Error("expected the embedded default")
	}
}

func TestReadPrefersTemplatesOnDisk(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "templates", "reset.tmpl"), []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	b, err := Read("reset.tmpl")
	if err != nil || string(b) != "custom" {
		t.Fatalf("got %q, %v; want the file on disk", b, err)
	}
}
