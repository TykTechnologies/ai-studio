package models

import (
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// CredentialRedacted is what an LLM credential is shown as outside the
// process (API responses, events, object hooks), and what a writer sends
// back to leave a stored credential unchanged.
const CredentialRedacted = "[redacted]"

// IsCredentialMetadataKey reports whether an LLM metadata key holds a
// credential: its lower-cased name contains "key", "secret" or "token"
// (Bedrock's aws_access_key_id, aws_secret_access_key and
// aws_session_token). The rule over-matches on purpose (cache_key_prefix,
// say): such a value is only encrypted at rest and redacted where it would
// leave the process, and used as written.
//
// One rule for every place that treats LLM credentials: encryption at rest,
// the API's and events' redaction, object hooks and what edges receive.
func IsCredentialMetadataKey(key string) bool {
	k := strings.ToLower(key)
	return strings.Contains(k, "key") || strings.Contains(k, "secret") || strings.Contains(k, "token")
}

// LLMCredentialValue reports whether v is a literal credential value, and
// returns it: a non-empty string that is not a $SECRET/ or $ENV/ reference
// and not CredentialRedacted. References, empty values and the redaction
// placeholder are never encrypted or redacted.
func LLMCredentialValue(v any) (string, bool) {
	s, ok := v.(string)
	if !ok || s == "" || s == CredentialRedacted || secrets.IsReference(s) {
		return "", false
	}
	return s, true
}

// RedactLLMCredentials is a copy of l for anything that leaves the process
// (system events, cluster_events, object hooks): a literal api_key and the
// literal values of credential metadata keys read CredentialRedacted, and
// references are kept, as the API shows them. l is not changed.
func RedactLLMCredentials(l *LLM) *LLM {
	if l == nil {
		return nil
	}
	out := *l
	if _, ok := LLMCredentialValue(out.APIKey); ok {
		out.APIKey = CredentialRedacted
	}
	// Its filters' guardrail connection secrets too (2ze).
	out.Filters = redactLLMFilters(l.Filters)
	if l.Metadata != nil {
		out.Metadata = make(JSONMap, len(l.Metadata))
		for k, v := range l.Metadata {
			if _, ok := LLMCredentialValue(v); ok && IsCredentialMetadataKey(k) {
				v = CredentialRedacted
			}
			out.Metadata[k] = v
		}
	}
	return &out
}

// RestoreLLMCredentials puts original's credentials back in modified (an
// object hook's result built from RedactLLMCredentials' copy) wherever it
// still reads CredentialRedacted: the redaction means "unchanged", as on
// the API. A credential the hook set to anything else is kept; one it
// removed stays removed.
func RestoreLLMCredentials(original, modified *LLM) {
	if original == nil || modified == nil {
		return
	}
	if modified.APIKey == CredentialRedacted {
		modified.APIKey = original.APIKey
	}
	for k, v := range modified.Metadata {
		if s, ok := v.(string); ok && s == CredentialRedacted {
			if orig, has := original.Metadata[k]; has {
				modified.Metadata[k] = orig
			} else {
				delete(modified.Metadata, k)
			}
		}
	}
}

// LLM credentials at rest (aa5): a literal api_key and the literal values of
// credential metadata keys are stored as secrets.EncryptValue output ($ENC/)
// when TYK_AI_SECRET_KEY is set, and as written without it. The hooks below
// do it for every gorm write and read of an LLM, so every reader sees
// plaintext: BeforeSave encrypts, AfterSave and AfterFind decrypt. A column
// or map update (Update("metadata", ...)) runs no struct hook: the only one,
// UpdateLLMMetadata, encrypts itself (EncryptLLMMetadata), and
// TestLLMCredentialColumnsAreOnlySavedWhole keeps it that way.

// llmCipher is a stored credential's ciphertext and a digest of the
// plaintext it decrypts to.
type llmCipher struct {
	sum    [sha256.Size]byte
	cipher string
}

type llmCipherKey struct {
	id    uint
	field string // "api_key", or "metadata." and the key
}

// llmCiphers remembers the ciphertext each credential was last read from or
// written as, by LLM and field, so a save that leaves a credential unchanged
// writes the same ciphertext (EncryptValue's is random): an unchanged LLM
// saves an unchanged row. Rows are few; deleted LLMs are forgotten.
var llmCiphers sync.Map // llmCipherKey -> llmCipher

const apiKeyField = "api_key"

func metadataField(key string) string { return "metadata." + key }

// encryptLLMCredential is plain as stored: the ciphertext it was read from
// when unchanged, else a new one (plain when no key is set).
func encryptLLMCredential(id uint, field, plain string) string {
	if id != 0 {
		if c, ok := llmCiphers.Load(llmCipherKey{id, field}); ok && c.(llmCipher).sum == sha256.Sum256([]byte(plain)) {
			return c.(llmCipher).cipher
		}
	}
	return secrets.EncryptValue(plain)
}

// decryptLLMCredential is stored as plaintext. A value that does not decrypt
// (no key, another key) reads as secrets.RefusedCredential, never as its
// ciphertext, which a vendor client would send as the key; it is logged
// without the value, and remembered so a save keeps the stored ciphertext.
func decryptLLMCredential(id uint, field, stored string) string {
	plain := secrets.DecryptValue(stored)
	if strings.HasPrefix(plain, encryptedPrefix) {
		logger.Errorf("LLM %d: the stored %s does not decrypt with TYK_AI_SECRET_KEY (unset or changed); it is refused until it is set again", id, field)
		plain = secrets.RefusedCredential
	}
	rememberLLMCredential(id, field, plain, stored)
	return plain
}

func rememberLLMCredential(id uint, field, plain, cipher string) {
	if id != 0 {
		llmCiphers.Store(llmCipherKey{id, field}, llmCipher{sum: sha256.Sum256([]byte(plain)), cipher: cipher})
	}
}

// forgetLLMCredentials drops what llmCiphers holds for a deleted LLM.
func forgetLLMCredentials(id uint) {
	llmCiphers.Range(func(k, _ any) bool {
		if k.(llmCipherKey).id == id {
			llmCiphers.Delete(k)
		}
		return true
	})
}

// encryptLLMCredentials puts l's literal credentials in their stored form,
// and reports whether it changed any. Metadata is replaced by a copy, so a
// map the caller shares is not changed.
func encryptLLMCredentials(l *LLM) bool {
	changed := false
	if v, ok := LLMCredentialValue(l.APIKey); ok && !strings.HasPrefix(v, encryptedPrefix) {
		l.APIKey = encryptLLMCredential(l.ID, apiKeyField, v)
		changed = l.APIKey != v
	}
	if l.Metadata == nil {
		return changed
	}
	var out JSONMap
	for k, raw := range l.Metadata {
		v, ok := LLMCredentialValue(raw)
		if !ok || !IsCredentialMetadataKey(k) || strings.HasPrefix(v, encryptedPrefix) {
			continue
		}
		if out == nil {
			out = make(JSONMap, len(l.Metadata))
			for k2, v2 := range l.Metadata {
				out[k2] = v2
			}
		}
		enc := encryptLLMCredential(l.ID, metadataField(k), v)
		out[k] = enc
		changed = changed || enc != v
	}
	if out != nil {
		l.Metadata = out
	}
	return changed
}

// decryptLLMCredentials puts l's stored credentials back in plaintext.
// Metadata is replaced by a copy, as in encryptLLMCredentials.
func decryptLLMCredentials(l *LLM) {
	if strings.HasPrefix(l.APIKey, encryptedPrefix) {
		l.APIKey = decryptLLMCredential(l.ID, apiKeyField, l.APIKey)
	}
	if l.Metadata == nil {
		return
	}
	var out JSONMap
	for k, raw := range l.Metadata {
		s, ok := raw.(string)
		if !ok || !strings.HasPrefix(s, encryptedPrefix) {
			continue
		}
		if out == nil {
			out = make(JSONMap, len(l.Metadata))
			for k2, v2 := range l.Metadata {
				out[k2] = v2
			}
		}
		out[k] = decryptLLMCredential(l.ID, metadataField(k), s)
	}
	if out != nil {
		l.Metadata = out
	}
}

// EncryptLLMMetadata is metadata with its literal credential values in
// their stored form, for a write that runs no LLM hook (a map update of the
// metadata column). The map passed in is not changed.
func EncryptLLMMetadata(id uint, metadata JSONMap) JSONMap {
	l := LLM{Metadata: metadata}
	l.ID = id
	encryptLLMCredentials(&l)
	return l.Metadata
}

// AfterFind decrypts the LLM's credentials (see encryptLLMCredentials).
func (l *LLM) AfterFind(tx *gorm.DB) error {
	decryptLLMCredentials(l)
	return nil
}

// BackfillLLMCredentialEncryption encrypts the literal credentials of every
// LLM row (soft-deleted ones included) written before they were encrypted at
// rest, or while no key was set. It does nothing without TYK_AI_SECRET_KEY.
// Each row is written with a compare-and-set on what was read, so an edit
// made meanwhile, or another replica's backfill, is never overwritten with
// stale values. Returns the number of rows encrypted.
func BackfillLLMCredentialEncryption(db *gorm.DB) (int, error) {
	if !secrets.EncryptionKeyConfigured() {
		return 0, nil
	}
	type row struct {
		ID       uint
		APIKey   string
		Metadata JSONMap
	}
	var rows []row
	if err := db.Session(&gorm.Session{NewDB: true}).Table("llms").Select("id", "api_key", "metadata").Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		l := LLM{APIKey: r.APIKey, Metadata: r.Metadata}
		l.ID = r.ID
		if !encryptLLMCredentials(&l) {
			continue
		}
		// Compared on api_key and on the metadata as read, as text (a json
		// column has no equality operator on Postgres).
		oldMeta, err := r.Metadata.Value()
		if err != nil {
			return n, err
		}
		q := db.Session(&gorm.Session{NewDB: true}).Table("llms").Where("id = ? AND api_key = ?", r.ID, r.APIKey)
		if oldMeta == nil {
			q = q.Where("metadata IS NULL")
		} else {
			q = q.Where("CAST(metadata AS TEXT) = ?", oldMeta)
		}
		res := q.UpdateColumns(map[string]any{"api_key": l.APIKey, "metadata": l.Metadata})
		if res.Error != nil {
			return n, res.Error
		}
		if res.RowsAffected > 0 {
			n++
		}
	}
	return n, nil
}
