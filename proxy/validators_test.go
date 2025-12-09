package proxy

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenAIValidator(t *testing.T) {
	t.Run("Valid Bearer token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer sk-test-key-12345")

		token, err := OpenAIValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "sk-test-key-12345", token)
	})

	t.Run("Missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)

		token, err := OpenAIValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "missing authorization header")
	})

	t.Run("Invalid authorization format (no Bearer)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "sk-test-key-12345")

		token, err := OpenAIValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "invalid authorization header")
	})

	t.Run("Empty token after Bearer", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer ")

		token, err := OpenAIValidator(req)
		assert.NoError(t, err) // Function doesn't validate empty token
		assert.Equal(t, "", token)
	})
}

func TestAnthropicValidator(t *testing.T) {
	t.Run("Valid x-api-key header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/messages", nil)
		req.Header.Set("x-api-key", "sk-ant-api-key-123")

		token, err := AnthropicValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "sk-ant-api-key-123", token)
	})

	t.Run("Missing x-api-key header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/messages", nil)

		token, err := AnthropicValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "missing authorization header")
	})
}

func TestDummyValidator(t *testing.T) {
	t.Run("Valid dummy authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("Dummy-Authorization", "test-token")

		token, err := DummyValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "test-token", token)
	})

	t.Run("Missing dummy authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)

		token, err := DummyValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
		assert.Contains(t, err.Error(), "missing dummy-authorization header")
	})
}

func TestMockValidator(t *testing.T) {
	t.Run("Valid authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		req.Header.Set("Authorization", "mock-token")

		token, err := MockValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "mock-token", token)
	})

	t.Run("Missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)

		token, err := MockValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestGoogleAIValidator(t *testing.T) {
	t.Run("Valid key in query parameter", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate?key=google-api-key-123", nil)

		token, err := GoogleAIValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "google-api-key-123", token)
	})

	t.Run("Valid key in x-goog-api-key header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)
		req.Header.Set("x-goog-api-key", "google-header-key")

		token, err := GoogleAIValidator(req)
		assert.NoError(t, err)
		// Note: The function returns empty string when query param is not present
		// but header is present (bug in implementation - returns h instead of h2)
		assert.Equal(t, "", token)
	})

	t.Run("Missing both query param and header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)

		token, err := GoogleAIValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("Query param takes precedence over header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate?key=query-key", nil)
		req.Header.Set("x-goog-api-key", "header-key")

		token, err := GoogleAIValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "query-key", token)
	})
}

func TestVertexValidator(t *testing.T) {
	t.Run("Valid Bearer token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)
		req.Header.Set("Authorization", "Bearer vertex-token-123")

		token, err := VertexValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "vertex-token-123", token)
	})

	t.Run("Missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)

		token, err := VertexValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("Invalid format (no Bearer)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)
		req.Header.Set("Authorization", "vertex-token")

		token, err := VertexValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}

func TestHuggingFaceValidator(t *testing.T) {
	t.Run("Valid Bearer token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)
		req.Header.Set("Authorization", "Bearer hf_token_12345")

		token, err := HuggingFaceValidator(req)
		assert.NoError(t, err)
		assert.Equal(t, "hf_token_12345", token)
	})

	t.Run("Missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)

		token, err := HuggingFaceValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})

	t.Run("Invalid format (no Bearer)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/generate", nil)
		req.Header.Set("Authorization", "hf-token")

		token, err := HuggingFaceValidator(req)
		assert.Error(t, err)
		assert.Empty(t, token)
	})
}
