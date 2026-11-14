package cluster

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Replicas of one cluster must share some secrets, and a mismatch shows up
// far from its cause: a form refused by the replica that did not issue its
// CSRF token, an edge that cannot decrypt its configuration, a secret that
// reads as garbage. Each replica therefore records a fingerprint of each
// such secret in its cluster_nodes row, and the cluster status compares the
// live replicas' fingerprints. Nothing refuses to start on a mismatch: a
// rolling rotation legitimately has replicas disagreeing for a while.

// The settings whose values replicas must share.
const (
	SettingCSRFKey                   = "CSRF_KEY"
	SettingSecretKey                 = "TYK_AI_SECRET_KEY"
	SettingMicrogatewayEncryptionKey = "MICROGATEWAY_ENCRYPTION_KEY"
	SettingGRPCAuthToken             = "GRPC_AUTH_TOKEN"
)

// fingerprintLabel keys the fingerprint HMAC. It is fixed: every replica
// must derive the same fingerprint from the same value.
const fingerprintLabel = "tyk-ai-studio/cluster/shared-secret-fingerprint/v1/"

// fingerprintBytes is how much of the HMAC is kept: enough that two
// different values practically never collide, and no more.
const fingerprintBytes = 4

// Fingerprint is what a replica records of the value of a shared setting:
// HMAC-SHA-256 keyed with a fixed label that includes the setting's name,
// truncated to 32 bits. It is not the value and not a plain hash of it: a
// table of hashes of likely keys does not match it, and the same value in
// two settings gives two fingerprints. Fingerprints are stored for
// comparison only and are never logged or returned by the API.
func Fingerprint(setting, value string) string {
	mac := hmac.New(sha256.New, []byte(fingerprintLabel+setting))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil)[:fingerprintBytes])
}

// SharedSecrets are the values of the shared settings as one replica has
// them. They stay in the process; only their fingerprints are recorded.
type SharedSecrets struct {
	// OwnCSRF: the replica issues Studio's own CSRF tokens (a full Studio
	// whose host has not replaced the CSRF protection). CSRFKey is then
	// compared, and an empty one means the key is generated at start.
	OwnCSRF bool
	CSRFKey string
	// SecretKey is compared as it is, empty or not: every replica reads
	// the same encrypted values.
	SecretKey string
	// MicrogatewayEncryptionKey and GRPCAuthToken are compared when set (a
	// replica that serves no edges sets neither).
	MicrogatewayEncryptionKey string
	GRPCAuthToken             string
}

// SecretFingerprints is what a replica records about its shared secrets.
// A nil field is not recorded and not compared.
type SecretFingerprints struct {
	CSRFKey    *string
	CSRFKeySet *bool
	SecretKey  *string
	// MicrogatewayEncryptionKey and GRPCAuthToken are nil when unset.
	MicrogatewayEncryptionKey *string
	GRPCAuthToken             *string
}

// Fingerprints returns what the replica records for s.
func (s SharedSecrets) Fingerprints() *SecretFingerprints {
	fp := func(setting, value string) *string {
		f := Fingerprint(setting, value)
		return &f
	}
	out := &SecretFingerprints{SecretKey: fp(SettingSecretKey, s.SecretKey)}
	if s.OwnCSRF {
		set := s.CSRFKey != ""
		out.CSRFKeySet = &set
		if set {
			out.CSRFKey = fp(SettingCSRFKey, s.CSRFKey)
		}
	}
	if s.MicrogatewayEncryptionKey != "" {
		out.MicrogatewayEncryptionKey = fp(SettingMicrogatewayEncryptionKey, s.MicrogatewayEncryptionKey)
	}
	if s.GRPCAuthToken != "" {
		out.GRPCAuthToken = fp(SettingGRPCAuthToken, s.GRPCAuthToken)
	}
	return out
}

// columns maps the cluster_nodes columns to their values (nil for NULL).
func (f SecretFingerprints) columns() map[string]interface{} {
	return map[string]interface{}{
		"csrf_key_fp":         f.CSRFKey,
		"csrf_key_set":        f.CSRFKeySet,
		"secret_key_fp":       f.SecretKey,
		"microgateway_key_fp": f.MicrogatewayEncryptionKey,
		"grpc_auth_token_fp":  f.GRPCAuthToken,
	}
}

// sharedSettings are compared in this order; effect says what breaks.
var sharedSettings = []struct {
	name   string
	fp     func(models.ClusterNode) *string
	effect string
}{
	{SettingCSRFKey, func(n models.ClusterNode) *string { return n.CSRFKeyFP },
		"a form loaded from one of them is refused by another"},
	{SettingSecretKey, func(n models.ClusterNode) *string { return n.SecretKeyFP },
		"secrets and keys encrypted by one of them cannot be read by another"},
	{SettingMicrogatewayEncryptionKey, func(n models.ClusterNode) *string { return n.MicrogatewayKeyFP },
		"an edge can decrypt the configuration of only the replicas that share its key"},
	{SettingGRPCAuthToken, func(n models.ClusterNode) *string { return n.GRPCAuthTokenFP },
		"an edge is refused by the replicas whose token is not its own (expected for a while during a rotation with GRPC_AUTH_TOKEN_NEXT)"},
}

// replicaName names a replica in a warning: its label and node ID, since
// replicas often share a label.
func replicaName(n models.ClusterNode) string {
	if n.Label == "" {
		return n.NodeID
	}
	return fmt.Sprintf("%s (%s)", n.Label, n.NodeID)
}

// secretWarnings compares the live replicas' recorded fingerprints: one
// warning per setting they disagree on, naming the setting and the
// replicas, and one when several replicas each generate their CSRF key.
// The fingerprints themselves never appear.
func secretWarnings(live []models.ClusterNode) []string {
	var warnings []string
	for _, setting := range sharedSettings {
		groups := map[string][]string{}
		for _, n := range live {
			if fp := setting.fp(n); fp != nil {
				groups[*fp] = append(groups[*fp], replicaName(n))
			}
		}
		if len(groups) < 2 {
			continue
		}
		sides := make([]string, 0, len(groups))
		for _, names := range groups {
			sort.Strings(names)
			sides = append(sides, strings.Join(names, ", "))
		}
		sort.Strings(sides)
		warnings = append(warnings, fmt.Sprintf("Replicas have different values of %s: %s. Until they match, %s.",
			setting.name, strings.Join(sides, " differ from "), setting.effect))
	}

	var issuing, generated []string
	for _, n := range live {
		if n.CSRFKeySet == nil {
			continue
		}
		issuing = append(issuing, replicaName(n))
		if !*n.CSRFKeySet {
			generated = append(generated, replicaName(n))
		}
	}
	if len(issuing) > 1 && len(generated) > 0 {
		sort.Strings(generated)
		warnings = append(warnings, fmt.Sprintf("%s is not set on %s: a replica without it generates its own key at start, so a form loaded from one replica is refused by another. Set the same %s on every replica.",
			SettingCSRFKey, strings.Join(generated, ", "), SettingCSRFKey))
	}
	return warnings
}

// SharedSecretWarnings reports where the live replicas disagree on a shared
// secret (see secretWarnings), for a replica to log when it joins. The
// cluster status carries the same warnings.
func SharedSecretWarnings(db *gorm.DB) ([]string, error) {
	live, err := LiveNodes(db)
	if err != nil {
		return nil, err
	}
	return secretWarnings(live), nil
}
