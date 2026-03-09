package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/captcha"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/gin-gonic/gin"
)

// captchaMiddleware verifies the CAPTCHA token from the request body before
// allowing the request through. The token is expected in a top-level
// "captcha_token" field in the JSON body.
//
// On verification failure the middleware returns 403 with a JSON error.
// On backend errors (provider unreachable) the request is allowed through
// (fail-open) so legitimate users are not blocked by transient issues.
func captchaMiddleware(provider captcha.Provider) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractCaptchaToken(c)
		if token == "" {
			c.JSON(http.StatusForbidden, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{
					{
						Title:  "CAPTCHA Required",
						Detail: "A valid CAPTCHA token is required for this request.",
					},
				},
			})
			c.Abort()
			return
		}

		err := provider.Verify(c.Request.Context(), token, c.ClientIP())
		if err != nil {
			if captcha.IsVerificationError(err) {
				c.JSON(http.StatusForbidden, ErrorResponse{
					Errors: []struct {
						Title  string `json:"title"`
						Detail string `json:"detail"`
					}{
						{
							Title:  "CAPTCHA Failed",
							Detail: "CAPTCHA verification failed. Please try again.",
						},
					},
				})
				c.Abort()
				return
			}
			// Fail open on backend/network errors
			logger.ErrorErr("captcha verification error (fail-open)", err)
		}

		c.Next()
	}
}

// extractCaptchaToken reads "captcha_token" from the JSON body and restores
// the body so downstream handlers can read it again.
func extractCaptchaToken(c *gin.Context) string {
	if c.Request.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var parsed struct {
		CaptchaToken string `json:"captcha_token"`
	}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return ""
	}
	return parsed.CaptchaToken
}
