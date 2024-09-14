package proxy

import (
	"fmt"
	"net/http"
	"strings"
)

func OpenAIValidator(r *http.Request) (string, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	split := strings.Split(h, "Bearer ")
	if len(split) != 2 {
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
