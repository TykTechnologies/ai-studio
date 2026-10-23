package bedrockVendor

import (
	"encoding/json"
	"net/http"
	"strings"
)

// ErrorTypeHeader is the header the AWS SDKs read a Bedrock error's type
// from (the AWS JSON protocol's X-Amzn-ErrorType).
const ErrorTypeHeader = "X-Amzn-ErrorType"

// maxErrorMessage bounds the message ErrorBody takes from a body that is not
// JSON.
const maxErrorMessage = 1 << 10

// ErrorType is the Bedrock runtime error type for an HTTP status, as an AWS
// SDK raises it: AccessDeniedException for 401/403, ThrottlingException for
// 429, ResourceNotFoundException for 404, ServiceUnavailableException for
// 503, ModelTimeoutException for 408, InternalServerException for other 5xx
// and ValidationException for other 4xx.
func ErrorType(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "AccessDeniedException"
	case http.StatusNotFound:
		return "ResourceNotFoundException"
	case http.StatusRequestTimeout:
		return "ModelTimeoutException"
	case http.StatusTooManyRequests:
		return "ThrottlingException"
	case http.StatusServiceUnavailable:
		return "ServiceUnavailableException"
	}
	if status >= http.StatusInternalServerError {
		return "InternalServerException"
	}
	return "ValidationException"
}

// ErrorBody is body as an AWS JSON protocol error ({"message": ...}), the
// shape an AWS SDK reads the message of a Bedrock error from. The message
// is the body's own: an {"error":{"message":...}} envelope (OpenAI's, a
// gateway plugin's), {"error":"..."}, {"message"|"Message":...}, else the
// body's text.
func ErrorBody(body []byte) []byte {
	out, _ := json.Marshal(map[string]string{"message": errorMessage(body)})
	return out
}

func errorMessage(body []byte) string {
	var doc struct {
		Error        json.RawMessage `json:"error"`
		Message      string          `json:"message"`
		MessageUpper string          `json:"Message"`
	}
	if json.Unmarshal(body, &doc) == nil {
		var inner struct {
			Message string `json:"message"`
		}
		var text string
		switch {
		case json.Unmarshal(doc.Error, &inner) == nil && inner.Message != "":
			return inner.Message
		case json.Unmarshal(doc.Error, &text) == nil && text != "":
			return text
		case doc.Message != "":
			return doc.Message
		case doc.MessageUpper != "":
			return doc.MessageUpper
		}
	}
	msg := strings.TrimSpace(string(body))
	if len(msg) > maxErrorMessage {
		msg = msg[:maxErrorMessage]
	}
	if msg == "" {
		msg = "the request was refused"
	}
	return msg
}
