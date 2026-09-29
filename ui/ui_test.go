package ui

import (
	"io/fs"
	"testing"
)

func TestFSHasAnIndexPage(t *testing.T) {
	if _, err := fs.Stat(FS, "index.html"); err != nil {
		t.Fatalf("index.html missing from the embedded frontend (Embedded=%v): %v", Embedded, err)
	}
}
