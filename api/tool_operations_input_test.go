package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func operationRequest(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/4/operations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c
}

func TestBindOperationName(t *testing.T) {
	t.Run("JSON:API envelope", func(t *testing.T) {
		got, err := bindOperationName(operationRequest(`{"data":{"attributes":{"operation":"getExchangeRate"}}}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "getExchangeRate" {
			t.Errorf("got %q", got)
		}
	})

	// The natural reading of the endpoint name, and what scripting the API
	// tends to produce. It used to bind to an empty OperationInput, and the
	// handler returned 200 with the tool unchanged.
	t.Run("bare form", func(t *testing.T) {
		got, err := bindOperationName(operationRequest(`{"operation":"getExchangeRate"}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "getExchangeRate" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("envelope wins when both are present", func(t *testing.T) {
		got, err := bindOperationName(operationRequest(
			`{"operation":"bare","data":{"attributes":{"operation":"enveloped"}}}`))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "enveloped" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("a body naming no operation is rejected", func(t *testing.T) {
		for _, body := range []string{
			`{}`,
			`{"operation":""}`,
			`{"data":{"attributes":{}}}`,
			`{"data":{"attributes":{"operation":""}}}`,
			`{"operationId":"getExchangeRate"}`,
		} {
			_, err := bindOperationName(operationRequest(body))
			if err == nil {
				t.Errorf("expected %s to be rejected", body)
				continue
			}
			// The message has to name both accepted shapes, since silently
			// doing nothing is what made this hard to spot.
			if !strings.Contains(err.Error(), `{"operation"`) ||
				!strings.Contains(err.Error(), `"data"`) {
				t.Errorf("error for %s should name both shapes, got %q", body, err.Error())
			}
		}
	})

	t.Run("malformed JSON is rejected", func(t *testing.T) {
		if _, err := bindOperationName(operationRequest(`not json`)); err == nil {
			t.Error("expected malformed JSON to be rejected")
		}
	})
}
