package services

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

// An edge applies its first snapshot before its service container exists;
// the key it carried is installed once the container sets the crypto, since
// an identical snapshot is not applied again (jyx.163).
func TestAgentTracingKey_ArrivesBeforeCrypto(t *testing.T) {
	t.Cleanup(func() {
		agenttrace.SetFingerprinter(nil)
		agentTracingKeyCrypto.Store(cryptoHolder{})
		pendingAgentTracingKey.Store(nil)
	})
	agentTracingKeyCrypto.Store(cryptoHolder{})
	key := bytes.Repeat([]byte{0x4D}, 32)
	installAgentTracingKey(&pb.AgentTracingKeys{KeyId: 9, HmacKeyEncrypted: encryptedTracingKey(t, key)})
	assert.Nil(t, agenttrace.CurrentFingerprinter(), "no crypto yet")

	setAgentTracingKeyCrypto(NewCryptoService(agentTracingTestEncKey))
	fp := agenttrace.CurrentFingerprinter()
	require.NotNil(t, fp, "installed when the crypto arrives")
	hub := agenttrace.StaticFingerprinter{ID: 9, Key: key}
	assert.Equal(t, hub.Fingerprint([]byte("x")), fp.Fingerprint([]byte("x")))
}

// The hub-spoke container, the one a real edge runs, sets the crypto too.
func TestHubSpokeContainer_SetsAgentTracingKeyCrypto(t *testing.T) {
	t.Cleanup(func() {
		agenttrace.SetFingerprinter(nil)
		agentTracingKeyCrypto.Store(cryptoHolder{})
		pendingAgentTracingKey.Store(nil)
	})
	agentTracingKeyCrypto.Store(cryptoHolder{})
	db := setupEdgeSyncTestDB(t)
	cfg := &config.Config{Security: config.SecurityConfig{EncryptionKey: agentTracingTestEncKey}}
	cfg.HubSpoke.Mode = "standalone"
	c, err := NewHubSpokeServiceContainer(db, cfg)
	require.NoError(t, err)
	t.Cleanup(c.Cleanup)
	h, _ := agentTracingKeyCrypto.Load().(cryptoHolder)
	assert.NotNil(t, h.c)
}
