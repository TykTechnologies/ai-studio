package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An LLM whose API key cannot be encrypted for edges is left out of the
// snapshot, as tools, datasources, embedders and tokens are: the key must
// never reach an edge in plaintext.
func TestSnapshotExcludesLLMWhoseKeyCannotBeEncrypted(t *testing.T) {
	server, db := setupTestServer(t, nil)
	t.Cleanup(server.Stop)
	llms := createTestLLMs(db, "")

	// A key encryptForMicrogateway refuses (wrong length).
	server.encryptionKey = "too-short"
	t.Setenv("MICROGATEWAY_ENCRYPTION_KEY", "")

	snapshot, err := server.getConfigurationSnapshot("")
	require.NoError(t, err)
	for _, llm := range snapshot.Llms {
		for _, created := range llms {
			assert.NotEqual(t, created.Name, llm.Name, "LLM %q was sent although its key could not be encrypted", llm.Name)
		}
		assert.NotContains(t, []string{"test-key-1", "test-key-2"}, llm.ApiKeyEncrypted, "plaintext API key in the snapshot")
	}
}
