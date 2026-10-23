package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// rawLLMCredentials is an LLM's credential columns as stored, read without
// the LLM's hooks.
func rawLLMCredentials(t *testing.T, db *gorm.DB, id uint) (string, models.JSONMap) {
	t.Helper()
	var r struct {
		APIKey   string
		Metadata models.JSONMap
	}
	require.NoError(t, db.Session(&gorm.Session{NewDB: true}).Table("llms").Select("api_key", "metadata").Where("id = ?", id).Take(&r).Error)
	return r.APIKey, r.Metadata
}

// LLM credentials are encrypted at rest (aa5), and every reader the design
// lists still gets plaintext: the service's getters (with and without
// reference resolution), a Chat that preloads its LLM, and the map update
// UpdateLLMMetadata makes encrypts too.
func TestLLMCredentialsAtRest_ReadersGetPlaintext(t *testing.T) {
	t.Setenv("TYK_AI_SECRET_KEY", "credentials-unit-test-key")
	db := setupDependentsTestDB(t)
	s := NewService(db)
	const key, secret = "sk-fake-unit-test-key", "fake/unit+test/secret/access/key"

	llm, err := s.CreateLLM("Bedrock", key, "us-east-1", 50, "", "", "", models.BEDROCK, true,
		nil, "anthropic.claude-3", nil, nil, nil, false,
		models.JSONMap{"aws_access_key_id": "AKIAFAKEUNITTEST0000", "aws_secret_access_key": secret}, nil)
	require.NoError(t, err)
	assert.Equal(t, key, llm.APIKey)

	storedKey, storedMeta := rawLLMCredentials(t, db, llm.ID)
	assert.True(t, strings.HasPrefix(storedKey, "$ENC/"))
	assert.True(t, strings.HasPrefix(storedMeta["aws_secret_access_key"].(string), "$ENC/"))

	byID, err := s.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, key, byID.APIKey)
	assert.Equal(t, secret, byID.Metadata["aws_secret_access_key"])

	byName, err := s.GetLLMByName("Bedrock")
	require.NoError(t, err)
	assert.Equal(t, key, byName.APIKey)
	assert.Equal(t, secret, byName.Metadata["aws_secret_access_key"])

	active, err := s.GetActiveLLMs()
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, key, active[0].APIKey)
	assert.Equal(t, secret, active[0].Metadata["aws_secret_access_key"])

	chat := &models.Chat{Name: "Chat", LLMID: llm.ID}
	require.NoError(t, db.Create(chat).Error)
	var withLLM models.Chat
	require.NoError(t, db.Preload("LLM").First(&withLLM, chat.ID).Error)
	require.NotNil(t, withLLM.LLM)
	assert.Equal(t, key, withLLM.LLM.APIKey, "a preload decrypts")
	assert.Equal(t, secret, withLLM.LLM.Metadata["aws_secret_access_key"])

	require.NoError(t, s.UpdateLLMMetadata(llm.ID, models.JSONMap{"aws_secret_access_key": "another/fake/secret"}))
	_, storedMeta = rawLLMCredentials(t, db, llm.ID)
	assert.True(t, strings.HasPrefix(storedMeta["aws_secret_access_key"].(string), "$ENC/"), "the map update encrypts")
	byID, err = s.GetLLMByID(llm.ID)
	require.NoError(t, err)
	assert.Equal(t, "another/fake/secret", byID.Metadata["aws_secret_access_key"])
}
