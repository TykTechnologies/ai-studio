package switches

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionCapabilities(t *testing.T) {
	assert.Equal(t, []models.Vendor{models.OPENAI, models.SYSTEMONE}, DecisionVendors())

	assert.True(t, SupportsDecisions(models.OPENAI))
	assert.True(t, SupportsDecisions(models.SYSTEMONE))
	assert.False(t, SupportsDecisions(models.ANTHROPIC))
	assert.False(t, SupportsDecisions("nope"))

	assert.Equal(t, "/v1/decisions", DecisionEndpoint(models.OPENAI))
	assert.Equal(t, "/v1/systemone", DecisionEndpoint(models.SYSTEMONE))

	assert.False(t, SupportsChat(models.SYSTEMONE))
	assert.True(t, SupportsChat(models.OPENAI))
	assert.True(t, SupportsChat("unknown"), "unknown vendors keep their own errors")

	assert.Equal(t, []string{"chat", "embeddings", "decisions"}, VendorCapabilities(models.OPENAI))
	assert.Equal(t, []string{"decisions"}, VendorCapabilities(models.SYSTEMONE))
	assert.Equal(t, []string{"chat"}, VendorCapabilities(models.ANTHROPIC))

	// A decisions-only vendor is not offered as an embedder or chat driver.
	assert.NotContains(t, EmbeddingVendors(), models.SYSTEMONE)
	assert.NotContains(t, AVAILABLE_LLM_DRIVERS, models.SYSTEMONE)
	_, err := FetchDriver(&models.LLM{Vendor: models.SYSTEMONE}, nil, nil, nil)
	assert.Error(t, err)
}

func TestUpstreamEndpoint_SystemOneDefault(t *testing.T) {
	assert.Equal(t, "https://api.typesafe.ai", UpstreamEndpoint(&models.LLM{Vendor: models.SYSTEMONE}))
	assert.Equal(t, "http://llama:8080", UpstreamEndpoint(&models.LLM{Vendor: models.SYSTEMONE, APIEndpoint: "http://llama:8080"}))
	assert.Equal(t, "", UpstreamEndpoint(&models.LLM{Vendor: models.OPENAI}))
}

func TestDetectStreamingIntent_SystemOneNeverStreams(t *testing.T) {
	r := httptest.NewRequest("POST", "/llm/call/jev/v1/systemone", strings.NewReader(`{"stream":true}`))
	streaming, err := DetectStreamingIntent(models.SYSTEMONE, r)
	require.NoError(t, err)
	assert.False(t, streaming)
}

func TestExtractResponseText_Decisions(t *testing.T) {
	openai := `{"model":"gpt-6-luna","answers":[{"type":"predicate","name":"angry","probability":0.8},{"type":"refusal","name":"urgency"}]}`
	text, err := ExtractResponseText(models.OPENAI, []byte(openai))
	require.NoError(t, err)
	assert.Equal(t, "angry: predicate p=0.8\nurgency: refused", text)

	systemone := `{"answers":{"topic":{"type":"choice","choice":"billing","confidence":0.9}}}`
	text, err = ExtractResponseText(models.SYSTEMONE, []byte(systemone))
	require.NoError(t, err)
	assert.Equal(t, "topic: choice=billing (confidence 0.9)", text)

	_, err = ExtractResponseText(models.SYSTEMONE, []byte(`{"error":"overloaded"}`))
	assert.Error(t, err)

	// Chat bodies are unaffected, including one that happens to carry an
	// "answers" field of its own.
	text, err = ExtractResponseText(models.OPENAI, []byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
	text, err = ExtractResponseText(models.OPENAI, []byte(`{"answers":[],"choices":[{"message":{"content":"hi"}}]}`))
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
	text, err = ExtractResponseText(models.OLLAMA, []byte(`{"answers":{},"choices":[{"message":{"content":"hi"}}]}`))
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
}
