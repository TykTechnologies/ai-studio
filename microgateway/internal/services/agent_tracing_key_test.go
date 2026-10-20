package services

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

const agentTracingTestEncKey = "0123456789abcdef0123456789abcdef"

func encryptedTracingKey(t *testing.T, key []byte) string {
	t.Helper()
	enc, err := NewCryptoService(agentTracingTestEncKey).Encrypt(base64.StdEncoding.EncodeToString(key))
	require.NoError(t, err)
	return enc
}

// D4: an applied snapshot installs the hub's fingerprint key, so the edge and
// the hub fingerprint the same bytes equal. A snapshot without the key (tracing
// off on the hub) removes it.
func TestEdgeSync_InstallsAgentTracingKey(t *testing.T) {
	setAgentTracingKeyCrypto(NewCryptoService(agentTracingTestEncKey))
	t.Cleanup(func() { agenttrace.SetFingerprinter(nil) })

	db := setupEdgeSyncTestDB(t)
	sync := NewEdgeSyncService(db, "ns")
	key := bytes.Repeat([]byte{0x3C}, 32)
	snap := &pb.ConfigurationSnapshot{
		Version:          "1",
		EdgeNamespace:    "ns",
		SnapshotTime:     timestamppb.Now(),
		AgentTracingKeys: &pb.AgentTracingKeys{KeyId: 77, HmacKeyEncrypted: encryptedTracingKey(t, key)},
	}
	require.NoError(t, sync.SyncConfiguration(snap))

	edge := agenttrace.CurrentFingerprinter()
	require.NotNil(t, edge)
	hub := agenttrace.StaticFingerprinter{ID: 77, Key: key}
	assert.Equal(t, hub.Fingerprint([]byte("canonical")), edge.Fingerprint([]byte("canonical")))

	off := &pb.ConfigurationSnapshot{Version: "2", EdgeNamespace: "ns", SnapshotTime: timestamppb.Now()}
	require.NoError(t, sync.SyncConfiguration(off))
	assert.Nil(t, agenttrace.CurrentFingerprinter(), "tracing off on the hub removes the key")
}

// A key that cannot be read is dropped, never half-installed: the edge then
// computes no fingerprints rather than wrong ones.
func TestInstallAgentTracingKey_Rejects(t *testing.T) {
	t.Cleanup(func() { agenttrace.SetFingerprinter(nil) })
	good := &pb.AgentTracingKeys{KeyId: 5, HmacKeyEncrypted: encryptedTracingKey(t, bytes.Repeat([]byte{1}, 32))}

	setAgentTracingKeyCrypto(NewCryptoService(agentTracingTestEncKey))
	installAgentTracingKey(good)
	require.NotNil(t, agenttrace.CurrentFingerprinter())

	cases := map[string]*pb.AgentTracingKeys{
		"nil":          nil,
		"zero id":      {KeyId: 0, HmacKeyEncrypted: good.HmacKeyEncrypted},
		"id over 255":  {KeyId: 256, HmacKeyEncrypted: good.HmacKeyEncrypted},
		"not base64":   {KeyId: 5, HmacKeyEncrypted: "%%%"},
		"tampered":     {KeyId: 5, HmacKeyEncrypted: good.HmacKeyEncrypted[:len(good.HmacKeyEncrypted)-4] + "AAAA"},
		"short key":    {KeyId: 5, HmacKeyEncrypted: encryptedTracingKey(t, []byte("short"))},
		"empty cipher": {KeyId: 5},
	}
	for name, k := range cases {
		installAgentTracingKey(good)
		installAgentTracingKey(k)
		assert.Nil(t, agenttrace.CurrentFingerprinter(), name)
	}

	// Another installation's microgateway key cannot decrypt it.
	installAgentTracingKey(good)
	setAgentTracingKeyCrypto(NewCryptoService("fedcba9876543210fedcba9876543210"))
	installAgentTracingKey(good)
	assert.Nil(t, agenttrace.CurrentFingerprinter())
}
