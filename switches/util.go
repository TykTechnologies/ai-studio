package switches

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func CopyRequestBody(r *http.Request) ([]byte, error) {
	// Check if the body is nil
	if r.Body == nil {
		return nil, nil
	}

	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	// Restore the io.ReadCloser to its original state
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	// Return the copied body
	return body, nil
}

func ExtractModelName(urlString string) (string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	// Split the path into segments
	segments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")

	// Find the index of "models" in the path
	modelsIndex := -1
	for i, segment := range segments {
		if segment == "models" {
			modelsIndex = i
			break
		}
	}

	// If "models" is not found or it's the last segment, return an error
	if modelsIndex == -1 || modelsIndex == len(segments)-1 {
		return "", fmt.Errorf("invalid URL format: 'models' not found or no model specified")
	}

	// Extract the model name (and repository if present)
	modelParts := segments[modelsIndex+1:]
	modelName := strings.Join(modelParts, "/")

	return modelName, nil
}

func extractModelIDFromVertexURL(url string) (string, error) {
	// Regular expression pattern to match the MODEL_ID at the end of the URL
	pattern := `/models/([^/]+)$`

	// Compile the regular expression
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to compile regex: %v", err)
	}

	// Find the first match in the URL
	match := re.FindStringSubmatch(url)

	if len(match) > 1 {
		// If a match is found, return the captured group (MODEL_ID)
		return match[1], nil
	}

	// If no match is found, return an error
	return "", fmt.Errorf("model ID not found in URL")
}

func extractModelIDFromGoogleURL(url string) (string, error) {
	// Regular expression pattern to match the MODEL-ID in the new URL format
	pattern := `/v1beta/models/([^/:]+)`

	// Compile the regular expression
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("failed to compile regex: %v", err)
	}

	// Find the first match in the URL
	match := re.FindStringSubmatch(url)

	if len(match) > 1 {
		// If a match is found, return the captured group (MODEL-ID)
		return match[1], nil
	}

	// If no match is found, return an error
	return "", fmt.Errorf("model ID not found in URL")
}
