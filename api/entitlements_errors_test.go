package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An entitlements refusal from a service reaches the client as 409 with
// errors[0].code QUOTA_EXCEEDED through the existing helper-error path, so
// the UI's 402/403 toasters leave it alone and the form shows the detail.
func TestExceededErrorReachesClientAs409QuotaExceeded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	msg := "Your organisation has reached its limit. Remove one to make room."
	r.POST("/x", func(c *gin.Context) {
		err := fmt.Errorf("create: %w", entitlements.NewExceededError(msg))
		if he, ok := asHelperError(err); ok {
			helpers.SendErrorResponse(c, he)
			return
		}
		helpers.SendErrorResponse(c, helpers.NewInternalServerError(err.Error()))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/x", nil))

	require.Equal(t, http.StatusConflict, w.Code)
	var body struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
			Code   string `json:"code"`
		} `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Errors, 1)
	assert.Equal(t, "QUOTA_EXCEEDED", body.Errors[0].Code)
	assert.Equal(t, msg, body.Errors[0].Detail)
}

// Errors without a code keep their old shape.
func TestErrorResponseWithoutCodeOmitsIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	helpers.SendErrorResponse(c, helpers.NewConflictError("taken"))

	assert.Equal(t, http.StatusConflict, w.Code)
	var body map[string][]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, has := body["errors"][0]["code"]
	assert.False(t, has)
}
