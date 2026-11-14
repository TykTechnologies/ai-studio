//go:build !enterprise

package studio

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/cluster"
)

func nodeRow(t *testing.T, opts Options, s *Studio) models.ClusterNode {
	t.Helper()
	var node models.ClusterNode
	require.NoError(t, opts.DB.First(&node, "node_id = ?", s.clusterNode.ID()).Error)
	return node
}

// A Studio records fingerprints of the secrets replicas must share: the
// secret key always, the edge key and token when set, and its CSRF key
// unless the host supplies the CSRF protection.
func TestStudioRecordsSharedSecretFingerprints(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts)
	require.NoError(t, err)
	node := nodeRow(t, opts, s)
	require.NotNil(t, node.SecretKeyFP)
	assert.Equal(t, cluster.Fingerprint(cluster.SettingSecretKey, opts.Config.SecretKey), *node.SecretKeyFP)
	require.NotNil(t, node.CSRFKeySet)
	assert.False(t, *node.CSRFKeySet, "CSRF_KEY is unset: the key is generated")
	assert.Nil(t, node.CSRFKeyFP)
	assert.Nil(t, node.MicrogatewayKeyFP)
	assert.Nil(t, node.GRPCAuthTokenFP)
	stopStudio(t, s)

	opts = newTestOptions(t)
	opts.Config.CSRFKey = "csrf-key"
	opts.Config.MicrogatewayEncryptionKey = "0123456789abcdef0123456789abcdef"
	opts.Config.GRPCAuthToken = "edge-token"
	s, err = New(opts)
	require.NoError(t, err)
	node = nodeRow(t, opts, s)
	require.NotNil(t, node.CSRFKeySet)
	assert.True(t, *node.CSRFKeySet)
	require.NotNil(t, node.CSRFKeyFP)
	assert.Equal(t, cluster.Fingerprint(cluster.SettingCSRFKey, "csrf-key"), *node.CSRFKeyFP)
	require.NotNil(t, node.MicrogatewayKeyFP)
	assert.Equal(t, cluster.Fingerprint(cluster.SettingMicrogatewayEncryptionKey, opts.Config.MicrogatewayEncryptionKey), *node.MicrogatewayKeyFP)
	require.NotNil(t, node.GRPCAuthTokenFP)
	assert.Equal(t, cluster.Fingerprint(cluster.SettingGRPCAuthToken, "edge-token"), *node.GRPCAuthTokenFP)
	stopStudio(t, s)

	opts = newTestOptions(t)
	opts.CSRF = func(next http.Handler) http.Handler { return next }
	s, err = New(opts)
	require.NoError(t, err)
	node = nodeRow(t, opts, s)
	assert.Nil(t, node.CSRFKeySet, "the host's CSRF protection is not Studio's to compare")
	assert.Nil(t, node.CSRFKeyFP)
	stopStudio(t, s)
}

// A replica that joins a cluster whose live replica has another
// MICROGATEWAY_ENCRYPTION_KEY starts all the same (a rotation has replicas
// disagreeing for a while) and says so at WARN, naming the setting and the
// replicas, without the fingerprints.
func TestStudioWarnsAtStartOnSecretMismatch(t *testing.T) {
	opts := newTestOptions(t)
	s, err := New(opts) // creates the schema
	require.NoError(t, err)
	stopStudio(t, s)

	peerKey := "fedcba9876543210fedcba9876543210"
	peer, err := cluster.StartNode(opts.DB, "peer-node", "test", cluster.NodeOptions{
		Label: "site-b",
		Secrets: cluster.SharedSecrets{
			OwnCSRF: true, SecretKey: opts.Config.SecretKey, MicrogatewayEncryptionKey: peerKey,
		}.Fingerprints(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { peer.Stop(context.Background()) })

	opts.NodeLabel = "site-a"
	opts.Config.MicrogatewayEncryptionKey = "0123456789abcdef0123456789abcdef"
	// New must succeed: a mismatch does not stop start-up.
	logs := startCapturingLogs(t, opts)

	var mismatch, csrf bool
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, `"level":"warn"`) {
			continue
		}
		if strings.Contains(line, "different values of MICROGATEWAY_ENCRYPTION_KEY") {
			mismatch = true
			assert.Contains(t, line, "site-a (")
			assert.Contains(t, line, "site-b (peer-node)")
		}
		if strings.Contains(line, "CSRF_KEY is not set on") {
			csrf = true
		}
	}
	assert.True(t, mismatch, "the mismatch is logged at WARN")
	assert.True(t, csrf, "two replicas generating their CSRF key are logged at WARN")
	for _, fp := range []string{
		cluster.Fingerprint(cluster.SettingMicrogatewayEncryptionKey, peerKey),
		cluster.Fingerprint(cluster.SettingMicrogatewayEncryptionKey, opts.Config.MicrogatewayEncryptionKey),
		cluster.Fingerprint(cluster.SettingSecretKey, opts.Config.SecretKey),
	} {
		assert.NotContains(t, logs, fp, "fingerprints are never logged")
	}
}
