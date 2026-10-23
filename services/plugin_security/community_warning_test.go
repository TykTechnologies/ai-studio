//go:build !enterprise
// +build !enterprise

package plugin_security

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/pkg/ociplugins"
)

// Signatures are required by default, so the Community Edition warning that
// it cannot verify them fires on every OCI-enabled start: once per process,
// however many services are built.
func TestCommunityRequireSignatureWarningOncePerProcess(t *testing.T) {
	requireSignatureWarning = sync.Once{}
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	t.Cleanup(func() { requireSignatureWarning = sync.Once{} })

	cfg := &Config{OCIConfig: &ociplugins.OCIConfig{RequireSignature: true}}
	for i := 0; i < 3; i++ {
		NewService(cfg)
	}
	if n := strings.Count(logs.String(), "signature verification is NOT available"); n != 1 {
		t.Errorf("warning logged %d times, want 1:\n%s", n, logs.String())
	}
}
