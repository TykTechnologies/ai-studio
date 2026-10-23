package models

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Obviously fake credentials: never real ones in tests.
const (
	fakeAPIKey    = "sk-fake-unit-test-key"
	fakeAccessKey = "AKIAFAKEUNITTEST0000"
	fakeSecretKey = "fake/unit+test/secret/access/key"
)

// storedLLM is an LLM's credential columns as stored, read without the
// LLM's hooks.
type storedLLM struct {
	APIKey   string
	Metadata JSONMap
}

func rawLLM(t *testing.T, db *gorm.DB, id uint) storedLLM {
	t.Helper()
	var r storedLLM
	require.NoError(t, db.Session(&gorm.Session{NewDB: true}).Table("llms").Select("api_key", "metadata").Where("id = ?", id).Take(&r).Error)
	return r
}

func bedrockLLM(name string) *LLM {
	return &LLM{Name: name, Vendor: BEDROCK, APIKey: fakeAPIKey, Metadata: JSONMap{
		"aws_access_key_id":     fakeAccessKey,
		"aws_secret_access_key": fakeSecretKey,
		"aws_session_token":     "$SECRET/AWS_SESSION", // a reference: never encrypted
		"aws_region":            "us-east-1",           // not a credential key
		"cache_key_prefix":      "",                    // empty: left alone
	}}
}

func isEnc(v any) bool {
	s, _ := v.(string)
	return strings.HasPrefix(s, "$ENC/")
}

// With a key, a literal api_key and the literal credential metadata values
// are stored encrypted; references, empty and non-credential values as
// written. Every read, and the struct after the save, has plaintext.
func TestLLMCredentialsEncryptedAtRest(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupTestDB(t)
	l := bedrockLLM("Bedrock")
	shared := l.Metadata
	require.NoError(t, l.Create(db))

	assert.Equal(t, fakeAPIKey, l.APIKey, "the caller's struct is back in plaintext")
	assert.Equal(t, fakeSecretKey, l.Metadata["aws_secret_access_key"])
	assert.Equal(t, fakeSecretKey, shared["aws_secret_access_key"], "the caller's map is never changed")

	raw := rawLLM(t, db, l.ID)
	assert.True(t, isEnc(raw.APIKey))
	assert.True(t, isEnc(raw.Metadata["aws_access_key_id"]))
	assert.True(t, isEnc(raw.Metadata["aws_secret_access_key"]))
	assert.Equal(t, "$SECRET/AWS_SESSION", raw.Metadata["aws_session_token"])
	assert.Equal(t, "us-east-1", raw.Metadata["aws_region"])
	assert.Equal(t, "", raw.Metadata["cache_key_prefix"])

	var got LLM
	require.NoError(t, db.First(&got, l.ID).Error)
	assert.Equal(t, fakeAPIKey, got.APIKey)
	assert.Equal(t, fakeAccessKey, got.Metadata["aws_access_key_id"])
	assert.Equal(t, fakeSecretKey, got.Metadata["aws_secret_access_key"])
	assert.Equal(t, "$SECRET/AWS_SESSION", got.Metadata["aws_session_token"])

	var all []LLM
	require.NoError(t, db.Find(&all).Error)
	require.Len(t, all, 1)
	assert.Equal(t, fakeAPIKey, all[0].APIKey, "a list read decrypts each row")

	// A partial select (the resource signing backfill's) decrypts too.
	var partial LLM
	require.NoError(t, db.Select("id", "api_key").First(&partial, l.ID).Error)
	assert.Equal(t, fakeAPIKey, partial.APIKey)
}

// Saving an unchanged LLM writes the same ciphertext (EncryptValue's is
// random), so the row does not change; a changed credential gets a new one.
func TestLLMCredentialsUnchangedSaveKeepsCiphertext(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupTestDB(t)
	l := bedrockLLM("Bedrock")
	require.NoError(t, l.Create(db))
	before := rawLLM(t, db, l.ID)

	var loaded LLM
	require.NoError(t, db.First(&loaded, l.ID).Error)
	loaded.ShortDescription = "edited"
	require.NoError(t, loaded.Update(db))
	after := rawLLM(t, db, l.ID)
	assert.Equal(t, before.APIKey, after.APIKey)
	assert.Equal(t, before.Metadata["aws_secret_access_key"], after.Metadata["aws_secret_access_key"])

	// The struct that was just saved, saved again: still unchanged.
	require.NoError(t, loaded.Update(db))
	assert.Equal(t, before.APIKey, rawLLM(t, db, l.ID).APIKey)

	loaded.Metadata = JSONMap{"aws_access_key_id": fakeAccessKey, "aws_secret_access_key": "another/fake/secret"}
	require.NoError(t, loaded.Update(db))
	changed := rawLLM(t, db, l.ID)
	assert.Equal(t, before.Metadata["aws_access_key_id"], changed.Metadata["aws_access_key_id"], "unchanged")
	assert.NotEqual(t, before.Metadata["aws_secret_access_key"], changed.Metadata["aws_secret_access_key"])
	assert.True(t, isEnc(changed.Metadata["aws_secret_access_key"]))
	var got LLM
	require.NoError(t, db.First(&got, l.ID).Error)
	assert.Equal(t, "another/fake/secret", got.Metadata["aws_secret_access_key"])
}

// Without a key, credentials are stored as written, as before (the
// existing warnings say so).
func TestLLMCredentialsWithoutAKeyStayAsWritten(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := setupTestDB(t)
	l := bedrockLLM("Bedrock")
	require.NoError(t, l.Create(db))
	raw := rawLLM(t, db, l.ID)
	assert.Equal(t, fakeAPIKey, raw.APIKey)
	assert.Equal(t, fakeSecretKey, raw.Metadata["aws_secret_access_key"])
}

// A credential that does not decrypt (the key unset or changed) reads as
// secrets.RefusedCredential, never as its ciphertext, and a save keeps the
// stored ciphertext, so the right key reads it again.
func TestLLMCredentialsThatDoNotDecryptAreRefused(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupTestDB(t)
	l := bedrockLLM("Bedrock")
	require.NoError(t, l.Create(db))
	stored := rawLLM(t, db, l.ID)

	t.Setenv("TYK_AI_SECRET_KEY", "another-unit-test-key")
	var got LLM
	require.NoError(t, db.First(&got, l.ID).Error)
	assert.Equal(t, secrets.RefusedCredential, got.APIKey)
	assert.Equal(t, secrets.RefusedCredential, got.Metadata["aws_secret_access_key"])
	assert.Equal(t, secrets.RefusedCredential, secrets.ForEdgeCredential(stored.APIKey), "ciphertext never goes to an edge")

	got.ShortDescription = "edited under the wrong key"
	require.NoError(t, got.Update(db))
	assert.Equal(t, stored.APIKey, rawLLM(t, db, l.ID).APIKey, "the stored ciphertext is kept")
	assert.Equal(t, stored.Metadata["aws_secret_access_key"], rawLLM(t, db, l.ID).Metadata["aws_secret_access_key"])

	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	var again LLM
	require.NoError(t, db.First(&again, l.ID).Error)
	assert.Equal(t, fakeAPIKey, again.APIKey)
}

// EncryptLLMMetadata (UpdateLLMMetadata's map update) encrypts the literal
// credential values of a copy.
func TestEncryptLLMMetadata(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	in := JSONMap{"aws_secret_access_key": fakeSecretKey, "aws_region": "eu-west-1", "aws_session_token": "$ENV/TOKEN"}
	out := EncryptLLMMetadata(7, in)
	assert.True(t, isEnc(out["aws_secret_access_key"]))
	assert.Equal(t, "eu-west-1", out["aws_region"])
	assert.Equal(t, "$ENV/TOKEN", out["aws_session_token"])
	assert.Equal(t, fakeSecretKey, in["aws_secret_access_key"], "the map passed in is not changed")
}

// The startup backfill encrypts rows written in plaintext (before 2.3, or
// while no key was set), once, and replicas running it together agree.
func TestBackfillLLMCredentialEncryption(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := setupTestDB(t)
	a, b := bedrockLLM("A"), bedrockLLM("B")
	require.NoError(t, a.Create(db))
	require.NoError(t, b.Create(db))
	ref := &LLM{Name: "Ref", Vendor: OPENAI, APIKey: "$SECRET/OPENAI_KEY"}
	require.NoError(t, ref.Create(db))

	n, err := BackfillLLMCredentialEncryption(db)
	require.NoError(t, err)
	assert.Zero(t, n, "nothing without a key")
	assert.Equal(t, fakeAPIKey, rawLLM(t, db, a.ID).APIKey)

	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	// One connection: each SQLite :memory: connection is its own database.
	// The two backfills still interleave statement by statement.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	var wg sync.WaitGroup
	counts := make([]int, 2)
	for i := range counts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := BackfillLLMCredentialEncryption(db)
			assert.NoError(t, err)
			counts[i] = c
		}(i)
	}
	wg.Wait()
	assert.GreaterOrEqual(t, counts[0]+counts[1], 2, "both Bedrock rows are encrypted, by one replica or the other")
	for _, id := range []uint{a.ID, b.ID} {
		raw := rawLLM(t, db, id)
		assert.True(t, isEnc(raw.APIKey))
		assert.True(t, isEnc(raw.Metadata["aws_secret_access_key"]))
		var got LLM
		require.NoError(t, db.First(&got, id).Error)
		assert.Equal(t, fakeAPIKey, got.APIKey)
		assert.Equal(t, fakeSecretKey, got.Metadata["aws_secret_access_key"])
	}
	assert.Equal(t, "$SECRET/OPENAI_KEY", rawLLM(t, db, ref.ID).APIKey, "references are never encrypted")

	n, err = BackfillLLMCredentialEncryption(db)
	require.NoError(t, err)
	assert.Zero(t, n, "idempotent")
}

// The backfill never overwrites a row that changed since it was read.
func TestBackfillLLMCredentialEncryptionComparesAndSets(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "")
	db := setupTestDB(t)
	a := bedrockLLM("A")
	require.NoError(t, a.Create(db))
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")

	// An edit lands between the backfill's read and its write.
	readDone := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:edit_after_read", func(tx *gorm.DB) {
		if readDone || tx.Statement.Table != "llms" {
			return
		}
		readDone = true
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE llms SET api_key = ? WHERE id = ?", "sk-fake-edited-key", a.ID).Error)
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:edit_after_read") })

	n, err := BackfillLLMCredentialEncryption(db)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Equal(t, "sk-fake-edited-key", rawLLM(t, db, a.ID).APIKey, "the edit is kept")
}

func TestIsCredentialMetadataKey(t *testing.T) {
	for key, want := range map[string]bool{
		"aws_access_key_id":     true,
		"aws_secret_access_key": true,
		"aws_session_token":     true,
		"API_KEY":               true,
		"cache_key_prefix":      true, // over-matches on purpose
		"aws_region":            false,
		"deployment":            false,
		"":                      false,
	} {
		assert.Equal(t, want, IsCredentialMetadataKey(key), key)
	}
}

func TestLLMCredentialValue(t *testing.T) {
	for v, want := range map[any]bool{
		"AKIAFAKEFAKEFAKE": true,
		"$ENC/abc":         true, // still a literal: never a reference
		"$SECRET/AWS_KEY":  false,
		"$ENV/AWS_KEY":     false,
		"[redacted]":       false,
		"":                 false,
		42:                 false,
		nil:                false,
	} {
		_, got := LLMCredentialValue(v)
		assert.Equal(t, want, got, "%v", v)
	}
}
