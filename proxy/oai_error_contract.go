package proxy

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

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

// driverEnvelopeMessage matches the langchaingo OpenAI and Anthropic clients'
// rendering of an upstream error whose body was an OpenAI envelope: they
// decode the envelope, keep only its message and append it to the status
// text. The suffix is therefore the inner envelope's message, verbatim.
var driverEnvelopeMessage = regexp.MustCompile(`API returned unexpected status code: \d{3}: (.+)$`)

// innerOAIError recovers the OpenAI error envelope behind a driver error.
// The loopback hop answers a policy block (and any other refusal) with a
// full envelope; the drivers keep its message only, as "API returned
// unexpected status code: 400: <message>", and re-wrapping that as our own
// error gave the caller the reason twice, nested in two prefixes. The
// returned APIError carries the message (and type/code when a driver
// quotes the whole envelope, which is tried first); anything that carries
// neither reports false and the caller keeps its own text.
func innerOAIError(err error) (*APIError, bool) {
	if err == nil {
		return nil, false
	}
	msg := err.Error()
	if start := strings.IndexByte(msg, '{'); start >= 0 {
		var envelope OAIErrorResponse
		if json.Unmarshal([]byte(strings.TrimSpace(msg[start:])), &envelope) == nil &&
			envelope.Error != nil && envelope.Error.Message != "" {
			return envelope.Error, true
		}
	}
	if m := driverEnvelopeMessage.FindStringSubmatch(strings.TrimSpace(msg)); m != nil {
		if inner := strings.TrimSpace(m[1]); inner != "" {
			return &APIError{Message: inner}, true
		}
	}
	return nil, false
}

// policyViolationBody is the proxy-log body for a request-filter block. The
// enterprise compliance service matches on the "policy_violation" substring,
// so the keys are fixed; the detail is marshalled, not printf'd, because a
// filter message may contain quotes.
func policyViolationBody(err error) []byte {
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	body, marshalErr := json.Marshal(map[string]string{"error": "policy_violation", "detail": detail})
	if marshalErr != nil {
		return []byte(`{"error":"policy_violation"}`)
	}
	return body
}

// parseLoopbackError reads a refusal body from the loopback hop as the error
// the client should be given. It understands the error envelope every vendor
// uses in one form or another ({"error":{"message":...}}: OpenAI's, the inner
// hop's own for a policy or budget refusal, Anthropic's and Google's), and the
// pass-through's own error body ({"status":...,"message":...,"error":"..."}),
// which the credential check and the upstream failure paths write. Anything
// else is left to the driver's error.
func parseLoopbackError(status int, body []byte) (*APIError, bool) {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, false
	}
	var inner struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Code    any     `json:"code"`
		Param   *string `json:"param"`
	}
	if json.Unmarshal(envelope.Error, &inner) == nil && inner.Message != "" {
		apiErr := &APIError{Message: inner.Message, Type: inner.Type, Param: inner.Param}
		// OpenAI's code is a string. Google puts the HTTP status there as a
		// number, which tells the caller nothing the status does not.
		if code, ok := inner.Code.(string); ok {
			apiErr.Code = code
		}
		return apiErr, true
	}
	var passThrough struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &passThrough) != nil || passThrough.Message == "" {
		return nil, false
	}
	message := passThrough.Message
	// The detail of a client error is the reason (which credential check or
	// budget failed); the detail of a server error is our internals.
	if status < http.StatusInternalServerError && passThrough.Error != "" && passThrough.Error != message {
		message += ": " + passThrough.Error
	}
	return &APIError{Message: message}, true
}

// writeOAIError writes apiErr at status, filling in the type and code the
// status implies when the error did not carry them.
func writeOAIError(w http.ResponseWriter, status int, apiErr *APIError) {
	if apiErr.Type == "" {
		apiErr.Type = oaiErrorType(status)
	}
	if apiErr.Code == nil || apiErr.Code == "" {
		apiErr.Code = oaiErrorCode(status)
	}
	apiErr.HTTPStatus = http.StatusText(status)
	apiErr.HTTPStatusCode = status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(OAIErrorResponse{Error: apiErr})
}

// respondAttemptFailure answers a request whose last attempt failed: with the
// refusal the loopback hop wrote when it could be read, otherwise as
// respondRelayingOAIError does from the driver's error.
func respondAttemptFailure(w http.ResponseWriter, f attemptFailure, message string) {
	if f.inner == nil {
		respondRelayingOAIError(w, f.status, message, f.err)
		return
	}
	inner := *f.inner
	slog.Error("api client error", "message", inner.Message, "status", f.status)
	writeOAIError(w, f.status, &inner)
}

// withLoopback takes the refusal the loopback hop answered the attempt with,
// when the relay could read one, as the failure's reason and status. The
// driver's own reading of it is what the waterfall had before; the status
// the inner hop wrote is the same or better. A timeout or a driver that could
// not be built is not a refusal and is left as it is.
func (f attemptFailure) withLoopback(relay *loopbackRelay) attemptFailure {
	if f.timedOut || f.driverError {
		return f
	}
	inner, status, ok := relay.failure()
	if !ok {
		return f
	}
	f.inner, f.status, f.hasStatus = inner, status, true
	return f
}
