package proxy

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"

	"github.com/tmc/langchaingo/llms"
)

// oaiErrorType maps an HTTP status to the error.type string OpenAI's contract
// requires. SDKs branch on this field to decide whether a failure is worth
// retrying, and every error we emitted carried "type":"" — which matches none
// of their cases, so they all fell through to a generic error.
func oaiErrorType(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "invalid_request_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	}
	if status >= 500 {
		return "api_error"
	}
	if status >= 400 {
		return "invalid_request_error"
	}
	return "api_error"
}

// oaiErrorCode is the machine-readable code. OpenAI types this as a string;
// emitting the numeric status here made SDKs that compare it to a string
// constant silently never match.
func oaiErrorCode(status int) string {
	switch status {
	case http.StatusNotFound:
		return "model_not_found"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusRequestEntityTooLarge:
		return "context_length_exceeded"
	}
	return oaiErrorType(status)
}

// upstreamStatusPatterns pull the vendor's HTTP status out of a driver error.
// The drivers expose it no other way - langchaingo formats failures as "API
// returned unexpected status code: 404: <message>" and Google's client as
// "googleapi: Error 429: ..." - so the alternative to reading it back out of
// the string is reporting every upstream refusal as our own 500.
var upstreamStatusPatterns = []*regexp.Regexp{
	regexp.MustCompile(`status(?: code)?:?\s*(\d{3})`),
	regexp.MustCompile(`\bError (\d{3})\b`),
}

// upstreamStatusFromError recovers the status the vendor actually returned,
// falling back to fallback when the error carries none.
//
// This is what stops an unknown model from surfacing as a 500. A 5xx tells the
// caller to retry something that can never succeed; the vendor said 404, and
// the client needs to see that.
func upstreamStatusFromError(err error, fallback int) (int, bool) {
	if err == nil {
		return fallback, false
	}

	// Prefer a typed error when a driver bothers to produce one.
	var llmErr *llms.Error
	if errors.As(err, &llmErr) {
		if status, ok := statusForErrorCode(llmErr.Code); ok {
			return status, true
		}
	}

	msg := err.Error()
	for _, re := range upstreamStatusPatterns {
		m := re.FindStringSubmatch(msg)
		if m == nil {
			continue
		}
		if status, convErr := strconv.Atoi(m[1]); convErr == nil && status >= 400 && status <= 599 {
			return status, true
		}
	}
	return fallback, false
}

func statusForErrorCode(code llms.ErrorCode) (int, bool) {
	switch code {
	case llms.ErrCodeAuthentication:
		return http.StatusUnauthorized, true
	case llms.ErrCodeRateLimit, llms.ErrCodeQuotaExceeded:
		return http.StatusTooManyRequests, true
	case llms.ErrCodeInvalidRequest, llms.ErrCodeContentFilter, llms.ErrCodeTokenLimit:
		return http.StatusBadRequest, true
	case llms.ErrCodeResourceNotFound:
		return http.StatusNotFound, true
	case llms.ErrCodeTimeout:
		return http.StatusGatewayTimeout, true
	case llms.ErrCodeProviderUnavailable:
		return http.StatusServiceUnavailable, true
	case llms.ErrCodeNotImplemented:
		return http.StatusNotImplemented, true
	}
	return 0, false
}

// bedrockErrorStatus recovers the HTTP status behind an AWS SDK error. Smithy
// wraps every service failure in a value exposing HTTPStatusCode(), so a
// "model not found" from Bedrock is a 404 we can pass through instead of the
// blanket 502 the caller used to get.
func bedrockErrorStatus(err error, fallback int) int {
	if err == nil {
		return fallback
	}
	var httpErr interface{ HTTPStatusCode() int }
	if errors.As(err, &httpErr) {
		if status := httpErr.HTTPStatusCode(); status >= 400 && status <= 599 {
			return status
		}
	}
	return fallback
}
