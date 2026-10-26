package models

import (
	"crypto/fips140"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Resource signing keys sign the resource IDs native API extensions hand out
// (pkg/nativeapi). The hub generates a random secret per LLM, so rotating the
// vendor credential no longer invalidates outstanding IDs; it travels to edges
// with the LLM config. A rotation keeps the previous secret verifiable until
// ResourceSigningPrevExpiresAt.

const (
	// ResourceSigningDerivedKeyPrefix marks a previous-key slot that holds
	// the key derived from the vendor credential the LLM had when it got its
	// first secret (base64url after the prefix): the key of every ID minted
	// before then. It is stored rather than re-derived, so changing the
	// vendor credential later keeps those IDs valid until the grace ends.
	ResourceSigningDerivedKeyPrefix = "derived-key:v1:"

	// ResourceSigningLegacyKey in the previous-key slot stands for the key
	// derived from the LLM's current vendor credential. Early backfills
	// wrote it; it is still read, but no longer written.
	ResourceSigningLegacyKey = "vendor-credential"

	// credentialKeyInfo is the HKDF info of keys derived from a vendor
	// credential (nativeapi.CredentialKey).
	credentialKeyInfo = "tyk-native-resource/v1"

	// DefaultResourceSigningGrace keeps the previous key verifiable after a
	// rotation (and the vendor-credential key after the upgrade backfill)
	// for longer than Anthropic keeps batch results (29 days).
	DefaultResourceSigningGrace = 30 * 24 * time.Hour
	// MaxResourceSigningGrace caps the grace an administrator can ask for.
	MaxResourceSigningGrace = 365 * 24 * time.Hour

	resourceSigningSecretBytes = 32
)

var resourceSigningColumns = []string{
	"resource_signing_secret",
	"resource_signing_secret_prev",
	"resource_signing_prev_expires_at",
	"resource_signing_rotated_at",
}

// ErrResourceSigningConflict: the LLM's signing key changed while it was
// being rotated. The caller retries.
var ErrResourceSigningConflict = errors.New("resource signing key changed concurrently")

// ErrResourceSigningGrace: a rotation's grace period is out of range.
var ErrResourceSigningGrace = fmt.Errorf("grace period must be between 0 and %s", MaxResourceSigningGrace)

// ResourceSigningKeys is an LLM's plaintext signing key material.
type ResourceSigningKeys struct {
	// Secret signs new IDs. Empty when the LLM has none (a standalone edge,
	// or a hub row not yet backfilled): IDs are then signed with the key
	// derived from the vendor credential.
	Secret string
	// Previous is still accepted until PreviousExpiresAt. It is a secret, a
	// stored vendor-credential key (ResourceSigningDerivedKeyPrefix), or
	// ResourceSigningLegacyKey.
	Previous          string
	PreviousExpiresAt *time.Time
	RotatedAt         *time.Time
}

// PreviousIsCredential reports whether the previous key is the one derived
// from a vendor credential (stored, or the legacy sentinel).
func (k ResourceSigningKeys) PreviousIsCredential() bool {
	return k.Previous == ResourceSigningLegacyKey || strings.HasPrefix(k.Previous, ResourceSigningDerivedKeyPrefix)
}

// minHKDFKeyBytes is the shortest HKDF input key FIPS 140-3 mode approves:
// 112 bits. Under GODEBUG=fips140=only crypto/hkdf refuses shorter keys.
const minHKDFKeyBytes = 112 / 8

// fipsEnabled is crypto/fips140.Enabled; tests replace it.
var fipsEnabled = fips140.Enabled

// HKDFKeyMaterial returns the HKDF input key for resource signing material
// (a vendor credential or a hub signing secret). Standard builds use the
// material as is, so their keys, and the IDs signed with them, never change.
// In FIPS 140-3 mode material shorter than 112 bits is hashed with SHA-256
// first, since the module does not approve (and under fips140=only refuses)
// shorter HKDF keys. Material of 14 bytes or more, which includes every
// hub-generated secret and real vendor key, derives the same key in every
// build; only a short credential derives a different one in FIPS mode.
func HKDFKeyMaterial(material string) []byte {
	if fipsEnabled() && len(material) < minHKDFKeyBytes {
		sum := sha256.Sum256([]byte(material))
		return sum[:]
	}
	return []byte(material)
}

// DeriveCredentialSigningKey derives the resource signing key of a vendor
// credential: HKDF-SHA256(credential, info "tyk-native-resource/v1"), with a
// short credential hashed first in FIPS mode (HKDFKeyMaterial).
func DeriveCredentialSigningKey(credential string) ([]byte, error) {
	if credential == "" {
		return nil, errors.New("empty credential")
	}
	return hkdf.Key(sha256.New, HKDFKeyMaterial(credential), nil, credentialKeyInfo, sha256.Size)
}

// storedCredentialKey is the previous-key value that keeps IDs signed with
// apiKey's derived key verifiable: the derived key itself, encrypted. "" when
// the LLM has no credential (it could not have handed out IDs).
func storedCredentialKey(apiKey string) (string, error) {
	credential := secrets.GetValue(apiKey, false)
	if credential == "" {
		return "", nil
	}
	key, err := DeriveCredentialSigningKey(credential)
	if err != nil {
		return "", err
	}
	return secrets.EncryptValue(ResourceSigningDerivedKeyPrefix + base64.RawURLEncoding.EncodeToString(key)), nil
}

// PreviousActive reports whether the previous key is still accepted at now.
func (k ResourceSigningKeys) PreviousActive(now time.Time) bool {
	return k.Previous != "" && k.PreviousExpiresAt != nil && now.Before(*k.PreviousExpiresAt)
}

// ResourceSigningKeys returns the LLM's signing keys in plaintext. The hub
// stores them encrypted; the edge holds them in plaintext already.
func (l *LLM) ResourceSigningKeys() ResourceSigningKeys {
	return ResourceSigningKeys{
		Secret:            secrets.DecryptValue(l.ResourceSigningSecret),
		Previous:          secrets.DecryptValue(l.ResourceSigningSecretPrev),
		PreviousExpiresAt: l.ResourceSigningPrevExpiresAt,
		RotatedAt:         l.ResourceSigningRotatedAt,
	}
}

// NewResourceSigningSecret returns a fresh random signing secret.
func NewResourceSigningSecret() (string, error) {
	b := make([]byte, resourceSigningSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BeforeCreate gives every new LLM its signing secret, whichever path
// creates it.
func (l *LLM) BeforeCreate(tx *gorm.DB) error {
	if l.ResourceSigningSecret != "" {
		return nil
	}
	secret, err := NewResourceSigningSecret()
	if err != nil {
		return fmt.Errorf("generate resource signing secret: %w", err)
	}
	now := time.Now().UTC()
	l.ResourceSigningSecret = secrets.EncryptValue(secret)
	l.ResourceSigningRotatedAt = &now
	return nil
}

// RotateLLMResourceSigningSecret replaces the LLM's signing secret. The old
// key stays verifiable for grace (0 revokes it at once). An LLM that had no
// secret yet keeps its vendor-credential key (stored, as by the backfill) for
// the grace period. The update
// is a compare-and-set on the old secret, so two concurrent rotations cannot
// both keep their own previous key: the loser gets
// ErrResourceSigningConflict.
func RotateLLMResourceSigningSecret(db *gorm.DB, id uint, grace time.Duration, now time.Time) error {
	if grace < 0 || grace > MaxResourceSigningGrace {
		return ErrResourceSigningGrace
	}
	var cur LLM
	if err := db.Select("id", "api_key", "resource_signing_secret").First(&cur, id).Error; err != nil {
		return err
	}
	secret, err := NewResourceSigningSecret()
	if err != nil {
		return fmt.Errorf("generate resource signing secret: %w", err)
	}
	now = now.UTC()
	updates := map[string]interface{}{
		"resource_signing_secret":          secrets.EncryptValue(secret),
		"resource_signing_secret_prev":     "",
		"resource_signing_prev_expires_at": nil,
		"resource_signing_rotated_at":      now,
	}
	if grace > 0 {
		prev := cur.ResourceSigningSecret // already encrypted at rest
		if prev == "" {
			// No secret yet: keep the vendor-credential key IDs were signed with.
			if prev, err = storedCredentialKey(cur.APIKey); err != nil {
				return fmt.Errorf("derive vendor-credential signing key: %w", err)
			}
		}
		if prev != "" {
			updates["resource_signing_secret_prev"] = prev
			updates["resource_signing_prev_expires_at"] = now.Add(grace)
		}
	}
	q := db.Model(&LLM{}).Where("id = ?", id)
	if cur.ResourceSigningSecret == "" {
		q = q.Where("(resource_signing_secret IS NULL OR resource_signing_secret = '')")
	} else {
		q = q.Where("resource_signing_secret = ?", cur.ResourceSigningSecret)
	}
	res := q.UpdateColumns(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrResourceSigningConflict
	}
	return nil
}

// BackfillLLMResourceSigningSecrets generates a signing secret for every LLM
// that has none (rows created before signing secrets existed). IDs those
// LLMs already handed out were signed with the vendor-credential key: the
// key derived from today's credential is stored as the previous key, so it
// stays verifiable for grace even if the credential changes. Each row is a compare-and-set, so replicas
// starting together generate one secret per LLM. Returns the rows updated.
func BackfillLLMResourceSigningSecrets(db *gorm.DB, grace time.Duration, now time.Time) (int, error) {
	var rows []LLM
	if err := db.Model(&LLM{}).Select("id", "api_key").
		Where("resource_signing_secret IS NULL OR resource_signing_secret = ''").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	now = now.UTC()
	n := 0
	for _, row := range rows {
		secret, err := NewResourceSigningSecret()
		if err != nil {
			return n, fmt.Errorf("generate resource signing secret: %w", err)
		}
		prev, err := storedCredentialKey(row.APIKey)
		if err != nil {
			return n, fmt.Errorf("derive vendor-credential signing key: %w", err)
		}
		var expires interface{}
		if prev != "" {
			expires = now.Add(grace)
		}
		res := db.Model(&LLM{}).
			Where("id = ? AND (resource_signing_secret IS NULL OR resource_signing_secret = '')", row.ID).
			UpdateColumns(map[string]interface{}{
				"resource_signing_secret":          secrets.EncryptValue(secret),
				"resource_signing_secret_prev":     prev,
				"resource_signing_prev_expires_at": expires,
				"resource_signing_rotated_at":      now,
			})
		if res.Error != nil {
			return n, res.Error
		}
		n += int(res.RowsAffected)
	}
	return n, nil
}
