package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

func OpenAIValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		slog.Error("missing authorization header", "package", "proxy", "function", "OpenAIValidator")
		return "", fmt.Errorf("missing authorization header")
	}

	split := strings.Split(h, "Bearer ")
	if len(split) != 2 {
		slog.Error("missing Bearer part", "package", "proxy", "function", "OpenAIValidator")
		return "", fmt.Errorf("invalid authorization header")
	}

	return split[1], nil
}

func AnthropicValidator(r *http.Request) (string, error) {
	h := r.Header.Get("x-api-key")
	if h == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	return h, nil
}

func DummyValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Dummy-Authorization")
	if h == "" {
		return "", fmt.Errorf("missing dummy-authorization header")
	}

	return h, nil
}

func MockValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	return h, nil
}

// GoogleAIValidator extracts and validates the API key from the incoming request.
// It checks both the 'x-goog-api-key' header and the 'key' query parameter.
func GoogleAIValidator(r *http.Request) (string, error) {
	headerKey := r.Header.Get("x-goog-api-key")
	queryKey := r.URL.Query().Get("key")

	// This strict validation prevents "Parameter Pollution" where a client might
	// unintentionally (or maliciously) send conflicting credentials.
	if headerKey != "" && queryKey != "" && headerKey != queryKey {
		return "", errors.New("ambiguous credentials: header and query keys do not match")
	}

	if headerKey != "" {
		return headerKey, nil
	}

	if queryKey != "" {
		return queryKey, nil
	}

	return "", errors.New("missing authorization: 'x-goog-api-key' header or 'key' query parameter")
}

func VertexValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	split := strings.Split(h, "Bearer ")
	if len(split) != 2 {
		fmt.Println("missing Bearer part")
		return "", fmt.Errorf("invalid authorization header")
	}

	return split[1], nil
}

func HuggingFaceValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	split := strings.Split(h, "Bearer ")
	if len(split) != 2 {
		fmt.Println("missing Bearer part")
		return "", fmt.Errorf("invalid authorization header")
	}

	return split[1], nil
}
