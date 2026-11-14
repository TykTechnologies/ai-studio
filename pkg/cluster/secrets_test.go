package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// forEachSecretsDB runs body on SQLite and, with DATABASE_URL, on Postgres.
func forEachSecretsDB(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	migrate := func(t *testing.T, db *gorm.DB) *gorm.DB {
		require.NoError(t, db.AutoMigrate(&models.ClusterLease{}, &models.EdgeInstance{}, &models.EdgePushCommand{}))
		return db
	}
	t.Run("sqlite", func(t *testing.T) { body(t, migrate(t, sqliteDB(t))) })
	t.Run("postgres", func(t *testing.T) { body(t, migrate(t, newTestCluster(t).replicaDB("secrets"))) })
}

// A fingerprint is the same on every replica for the same value, differs
// with the value and with the setting, and is neither the value nor a
// plain hash of it.
func TestFingerprint(t *testing.T) {
	const value = "0123456789abcdef0123456789abcdef"
	fp := Fingerprint(SettingMicrogatewayEncryptionKey, value)
	assert.Equal(t, fp, Fingerprint(SettingMicrogatewayEncryptionKey, value))
	assert.Len(t, fp, 2*fingerprintBytes)
	assert.NotEqual(t, fp, Fingerprint(SettingMicrogatewayEncryptionKey, value+"x"))
	assert.NotEqual(t, fp, Fingerprint(SettingSecretKey, value), "the same value in another setting")
	assert.NotContains(t, value, fp)
	plain := sha256.Sum256([]byte(value))
	assert.NotEqual(t, hex.EncodeToString(plain[:fingerprintBytes]), fp, "not a plain hash of the value")
	assert.Len(t, Fingerprint(SettingGRPCAuthToken, ""), 2*fingerprintBytes, "an empty value has a fingerprint too")
}

func startSecretsNode(t *testing.T, db *gorm.DB, id, label string, s SharedSecrets) *Node {
	t.Helper()
	n, err := StartNode(db, id, "test", NodeOptions{Label: label, Secrets: s.Fingerprints()})
	require.NoError(t, err)
	t.Cleanup(func() { n.Stop(context.Background()) })
	return n
}

func statusWarnings(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	st, err := Snapshot(context.Background(), db, "node-a", nil, nil)
	require.NoError(t, err)
	var secret []string
	for _, w := range st.Warnings {
		for _, s := range sharedSettings {
			if strings.Contains(w, s.name) {
				secret = append(secret, w)
				break
			}
		}
	}
	joined, err := SharedSecretWarnings(db)
	require.NoError(t, err)
	assert.ElementsMatch(t, secret, joined, "the start-up warnings are the cluster status's")
	return secret
}

// Two live replicas that disagree on a shared secret: the cluster status
// names the setting and both replicas, for each of the four settings, and
// says nothing once they agree.
func TestClusterStatusWarnsOnSecretMismatch(t *testing.T) {
	base := SharedSecrets{
		OwnCSRF: true, CSRFKey: "csrf-key-shared",
		SecretKey:                 "secret-key-shared",
		MicrogatewayEncryptionKey: "0123456789abcdef0123456789abcdef",
		GRPCAuthToken:             "edge-token-shared",
	}
	differ := map[string]func(*SharedSecrets){
		SettingCSRFKey:                   func(s *SharedSecrets) { s.CSRFKey = "csrf-key-other" },
		SettingSecretKey:                 func(s *SharedSecrets) { s.SecretKey = "secret-key-other" },
		SettingMicrogatewayEncryptionKey: func(s *SharedSecrets) { s.MicrogatewayEncryptionKey = "fedcba9876543210fedcba9876543210" },
		SettingGRPCAuthToken:             func(s *SharedSecrets) { s.GRPCAuthToken = "edge-token-other" },
	}
	forEachSecretsDB(t, func(t *testing.T, db *gorm.DB) {
		a := startSecretsNode(t, db, "node-a", "site-a", base)
		assert.Empty(t, statusWarnings(t, db), "a single replica shows no warning")

		for setting, change := range differ {
			other := base
			change(&other)
			b := startSecretsNode(t, db, "node-b", "site-b", other)
			warnings := statusWarnings(t, db)
			require.Len(t, warnings, 1, setting)
			assert.Contains(t, warnings[0], setting)
			assert.Contains(t, warnings[0], "site-a (node-a)")
			assert.Contains(t, warnings[0], "site-b (node-b)")
			for _, n := range []*Node{a, b} {
				for _, fp := range n.secrets.columns() {
					if s, ok := fp.(*string); ok && s != nil {
						assert.NotContains(t, warnings[0], *s, "a warning never carries a fingerprint")
					}
				}
			}
			b.Stop(context.Background())
		}

		// The restarted replica with the shared values: its row is
		// rewritten and the warning goes.
		startSecretsNode(t, db, "node-b", "site-b", base)
		assert.Empty(t, statusWarnings(t, db))

		// The fingerprints are stored, and stay out of the status.
		var row models.ClusterNode
		require.NoError(t, db.First(&row, "node_id = ?", "node-a").Error)
		require.NotNil(t, row.MicrogatewayKeyFP)
		assert.Equal(t, Fingerprint(SettingMicrogatewayEncryptionKey, base.MicrogatewayEncryptionKey), *row.MicrogatewayKeyFP)
		require.NoError(t, a.beat(), "a refresh keeps them")
		require.NoError(t, db.First(&row, "node_id = ?", "node-a").Error)
		require.NotNil(t, row.SecretKeyFP)
		st, err := Snapshot(context.Background(), db, "node-a", nil, nil)
		require.NoError(t, err)
		out, err := json.Marshal(st)
		require.NoError(t, err)
		rowJSON, err := json.Marshal(row)
		require.NoError(t, err)
		for _, fp := range []*string{row.CSRFKeyFP, row.SecretKeyFP, row.MicrogatewayKeyFP, row.GRPCAuthTokenFP} {
			require.NotNil(t, fp)
			assert.NotContains(t, string(out), *fp)
			assert.NotContains(t, string(rowJSON), *fp)
		}
	})
}

// CSRF_KEY generated by more than one replica is a warning; the same key on
// each is not, nor is one replica on its own, nor a headless control plane
// next to one full Studio (it issues no tokens).
func TestClusterStatusWarnsOnGeneratedCSRFKey(t *testing.T) {
	forEachSecretsDB(t, func(t *testing.T, db *gorm.DB) {
		generated := SharedSecrets{OwnCSRF: true, SecretKey: "k"}
		a := startSecretsNode(t, db, "node-a", "site-a", generated)
		assert.Empty(t, statusWarnings(t, db), "one replica")

		headless := startSecretsNode(t, db, "node-h", "mdcb", SharedSecrets{SecretKey: "k"})
		assert.Empty(t, statusWarnings(t, db), "the only replica that issues tokens")
		headless.Stop(context.Background())

		b := startSecretsNode(t, db, "node-b", "site-b", generated)
		warnings := statusWarnings(t, db)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "CSRF_KEY is not set on site-a (node-a), site-b (node-b)")
		b.Stop(context.Background())

		// One replica has the key, the other still generates its own.
		b = startSecretsNode(t, db, "node-b", "site-b", SharedSecrets{OwnCSRF: true, CSRFKey: "shared", SecretKey: "k"})
		warnings = statusWarnings(t, db)
		require.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "CSRF_KEY is not set on site-a (node-a):")
		a.Stop(context.Background())

		startSecretsNode(t, db, "node-a", "site-a", SharedSecrets{OwnCSRF: true, CSRFKey: "shared", SecretKey: "k"})
		assert.Empty(t, statusWarnings(t, db), "the same CSRF_KEY on both")
	})
}

// A replica from before the columns existed (or one that records nothing)
// leaves them NULL and is left out of the comparison.
func TestClusterStatusIgnoresReplicasWithoutFingerprints(t *testing.T) {
	forEachSecretsDB(t, func(t *testing.T, db *gorm.DB) {
		startSecretsNode(t, db, "node-a", "site-a", SharedSecrets{OwnCSRF: true, SecretKey: "k", GRPCAuthToken: "t"})
		old, err := StartNode(db, "node-old", "test", NodeOptions{Label: "old"})
		require.NoError(t, err)
		t.Cleanup(func() { old.Stop(context.Background()) })
		var row models.ClusterNode
		require.NoError(t, db.First(&row, "node_id = ?", "node-old").Error)
		assert.Nil(t, row.SecretKeyFP)
		assert.Nil(t, row.CSRFKeySet)
		assert.Empty(t, statusWarnings(t, db))
	})
}
