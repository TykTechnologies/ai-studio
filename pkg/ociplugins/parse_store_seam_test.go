package ociplugins

import (
	"path/filepath"
	"testing"
)

// Both plugin managers take the same route: ParseOCICommand on a
// "oci://registry/repo@sha256:..." command, then the digest straight into
// content storage. The prefixed digest the parser yields must survive that
// hand-off, in the Studio and the microgateway alike.
func TestParsedDigestReachesStorage(t *testing.T) {
	s := newTestStorage(t)
	data := []byte("plugin-binary")
	bare := digestOf(data)

	command := "oci://ghcr.io/tyk/my-plugin@sha256:" + bare + "?arch=linux/amd64"
	ref, params, err := ParseOCICommand(command)
	if err != nil {
		t.Fatalf("ParseOCICommand: %v", err)
	}
	if ref.Digest != "sha256:"+bare {
		t.Fatalf("parser dropped the algorithm prefix: %q", ref.Digest)
	}

	execPath, err := s.StoreExecutable(ref.Digest, params.Architecture, data)
	if err != nil {
		t.Fatalf("StoreExecutable with parsed digest: %v", err)
	}
	if filepath.Base(execPath) != "sha256-"+bare+"-linux-amd64" {
		t.Errorf("unexpected executable filename: %s", filepath.Base(execPath))
	}

	// The cache-hit check both managers make before fetching must agree.
	if !s.HasExecutable(ref.Digest, params.Architecture) {
		t.Error("cached plugin not found under the digest the parser produced")
	}
}
