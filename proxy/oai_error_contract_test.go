package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
)

func TestOAIErrorType(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:            "invalid_request_error",
		http.StatusUnauthorized:          "authentication_error",
		http.StatusForbidden:             "permission_error",
		http.StatusNotFound:              "not_found_error",
		http.StatusTooManyRequests:       "rate_limit_error",
		http.StatusUnprocessableEntity:   "invalid_request_error",
		http.StatusInternalServerError:   "api_error",
		http.StatusBadGateway:            "api_error",
		http.StatusRequestEntityTooLarge: "invalid_request_error",
	}
	for status, want := range cases {
		assert.Equalf(t, want, oaiErrorType(status), "status %d", status)
	}
}

// The whole point of the type field is that a client can branch on it, which it
// cannot do when every error carries the same empty string.
func TestRespondWithOAIErrorFillsTheContract(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			w := httptest.NewRecorder()
			respondWithOAIError(w, status, "boom", nil, false)

			require.Equal(t, status, w.Code)
			var body struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    any    `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

			assert.NotEmpty(t, body.Error.Type, "OpenAI's contract requires a non-empty error.type")
			assert.Equal(t, oaiErrorType(status), body.Error.Type)
			assert.NotEmpty(t, body.Error.Message)
			assert.IsType(t, "", body.Error.Code, "OpenAI types error.code as a string, not a number")
		})
	}
}

func TestUpstreamStatusFromError(t *testing.T) {
	t.Run("reads the status out of a driver error", func(t *testing.T) {
		// This is the exact shape langchaingo produces; it exposes the upstream
		// status no other way, and without it an unknown model - a 404 - was
		// reported to the caller as our 500.
		err := fmt.Errorf("API returned unexpected status code: 404: model not found")
		status, ok := upstreamStatusFromError(err, http.StatusInternalServerError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("reads a wrapped driver error", func(t *testing.T) {
		err := fmt.Errorf("anthropic: %w", errors.New("API returned unexpected status code: 429"))
		status, _ := upstreamStatusFromError(err, http.StatusInternalServerError)
		assert.Equal(t, http.StatusTooManyRequests, status)
	})

	t.Run("reads a googleapi error", func(t *testing.T) {
		// Google's client formats its failures differently; a Gemini quota
		// refusal used to surface as our 500 and told the caller to retry
		// immediately instead of backing off.
		err := fmt.Errorf("failed to generate content: googleapi: Error 429: You exceeded your current quota")
		status, ok := upstreamStatusFromError(err, http.StatusInternalServerError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusTooManyRequests, status)
	})

	t.Run("prefers a typed error", func(t *testing.T) {
		err := llms.NewError(llms.ErrCodeAuthentication, "openai", "bad key")
		status, ok := upstreamStatusFromError(err, http.StatusInternalServerError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, status)
	})

	t.Run("falls back when the error says nothing", func(t *testing.T) {
		status, ok := upstreamStatusFromError(errors.New("connection reset"), http.StatusBadGateway)
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadGateway, status)
	})

	t.Run("ignores a number that is not a status", func(t *testing.T) {
		status, ok := upstreamStatusFromError(errors.New("status code: 999"), http.StatusBadGateway)
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadGateway, status)
	})

	t.Run("nil error", func(t *testing.T) {
		status, ok := upstreamStatusFromError(nil, http.StatusBadGateway)
		assert.False(t, ok)
		assert.Equal(t, http.StatusBadGateway, status)
	})
}

type statusErr struct{ code int }

func (e *statusErr) Error() string       { return "aws blew up" }
func (e *statusErr) HTTPStatusCode() int { return e.code }

func TestBedrockErrorStatus(t *testing.T) {
	assert.Equal(t, http.StatusNotFound,
		bedrockErrorStatus(fmt.Errorf("operation error: %w", &statusErr{code: 404}), http.StatusBadGateway))
	assert.Equal(t, http.StatusBadGateway,
		bedrockErrorStatus(errors.New("dial tcp: timeout"), http.StatusBadGateway))
	assert.Equal(t, http.StatusBadGateway, bedrockErrorStatus(nil, http.StatusBadGateway))
}
