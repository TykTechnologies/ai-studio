package services

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testVertexSAKey is a service account key JSON with a fresh RSA key.
var testVertexSAKey = func() string {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "sa@p.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"token_uri":   "https://oauth2.googleapis.com/token",
	})
	return string(b)
}()

func createVertexLLM(svc *Service, name, key, endpoint string) (*models.LLM, error) {
	return svc.CreateLLM(name, key, endpoint, 50, "", "", "", models.VERTEX, true, nil, "gemini-2.5-flash", nil, nil, nil, false, nil, nil)
}

func TestCreateLLM_VertexEndpointValidated(t *testing.T) {
	svc := NewService(setupTestDB(t))
	for i, bad := range []string{"proj", "a:b:c", "", "proj:us_central1"} {
		_, err := createVertexLLM(svc, "bad"+string(rune('a'+i)), testVertexSAKey, bad)
		require.Error(t, err, bad)
		var verr *LLMValidationError
		assert.True(t, errors.As(err, &verr), "a 400, not a 500: %v", err)
		assert.Contains(t, err.Error(), "project:location")
	}
	for i, ok := range []string{"my-proj:us-central1", "my-proj:global", "my-proj:eu", "my-proj:us", "example.com:my-proj:us-central1",
		"https://us-central1-aiplatform.googleapis.com", "http://127.0.0.1:8089/v1/projects/p/locations/us-central1"} {
		_, err := createVertexLLM(svc, "ok"+string(rune('a'+i)), testVertexSAKey, ok)
		require.NoError(t, err, ok)
	}
}

func TestCreateLLM_VertexKeyValidatedWithoutEcho(t *testing.T) {
	svc := NewService(setupTestDB(t))
	const secret = "s3cr3t-material"
	_, err := createVertexLLM(svc, "badkey", `{"type":"service_account","private_key":"`+secret+`"}`, "my-proj:us-central1")
	require.Error(t, err)
	var verr *LLMValidationError
	assert.True(t, errors.As(err, &verr))
	assert.NotContains(t, err.Error(), secret)

	_, err = createVertexLLM(svc, "garbage", "AIza"+secret, "my-proj:us-central1")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)

	// Key-less: saved (Studio chat may use host credentials), with a log warning.
	_, err = createVertexLLM(svc, "keyless", "", "my-proj:us-central1")
	require.NoError(t, err)
	// A secret reference is resolved later, not checked here.
	_, err = createVertexLLM(svc, "ref", "$SECRET/VERTEX_KEY", "my-proj:us-central1")
	require.NoError(t, err)
	// An access token is accepted.
	_, err = createVertexLLM(svc, "token", "ya29.test", "my-proj:us-central1")
	require.NoError(t, err)
}

func TestUpdateLLM_VertexStoredValuesNotRechecked(t *testing.T) {
	db := setupTestDB(t)
	svc := NewService(db)
	// An LLM saved before validation existed: an endpoint and a key the new
	// rules refuse.
	legacy := &models.LLM{Name: "legacy", Vendor: models.VERTEX, APIEndpoint: "a:b:c", APIKey: "old-unused-key", Active: true}
	require.NoError(t, db.Create(legacy).Error)

	update := func(name, key, endpoint string, active bool, allowed []string) error {
		_, err := svc.UpdateLLM(legacy.ID, name, key, endpoint, 0, "", "", "", models.VERTEX, active, nil, "", allowed, nil, nil, "", false, nil, nil)
		return err
	}
	require.NoError(t, update("legacy-renamed", "[redacted]", "a:b:c", true, nil), "rename keeps the stored endpoint and key")
	require.NoError(t, update("legacy-renamed", "[redacted]", "a:b:c", false, nil), "toggling active")
	require.NoError(t, update("legacy-renamed", "[redacted]", "a:b:c", false, []string{"gemini-*"}), "editing allowed models")

	require.Error(t, update("legacy-renamed", "still-not-a-credential", "a:b:c", false, nil), "a newly supplied key is checked")
	err := update("legacy-renamed", "[redacted]", "proj", false, nil)
	require.Error(t, err, "a changed endpoint is checked")
	var verr *LLMValidationError
	assert.True(t, errors.As(err, &verr))
	require.NoError(t, update("legacy-renamed", "[redacted]", "my-proj:us-central1", false, nil), "fixing the endpoint")

	// Switching another vendor's LLM to Vertex checks everything.
	oai := &models.LLM{Name: "oai", Vendor: models.OPENAI, APIEndpoint: "https://api.openai.com/v1", APIKey: "sk-x"}
	require.NoError(t, db.Create(oai).Error)
	_, err = svc.UpdateLLM(oai.ID, "oai", "[redacted]", "https://api.openai.com/v1", 0, "", "", "", models.VERTEX, true, nil, "", nil, nil, nil, "", false, nil, nil)
	require.Error(t, err, "the stored OpenAI key is not a Vertex credential")
}

func TestCreateLLM_OtherVendorsUnaffected(t *testing.T) {
	svc := NewService(setupTestDB(t))
	_, err := svc.CreateLLM("oai", "sk-x", "proj", 0, "", "", "", models.OPENAI, true, nil, "", nil, nil, nil, false, nil, nil)
	require.NoError(t, err)
}
