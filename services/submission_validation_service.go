package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/TykTechnologies/midsommar/v2/universalclient"
)

// SpecValidationError represents a single validation issue with a field reference
type SpecValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// SpecValidationWarning represents a non-blocking issue
type SpecValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// SpecValidationResult contains the full result of OAS spec validation
type SpecValidationResult struct {
	Valid      bool                    `json:"valid"`
	Errors     []SpecValidationError   `json:"errors"`
	Warnings   []SpecValidationWarning `json:"warnings"`
	Extracted  *SpecExtractedInfo      `json:"extracted,omitempty"`
}

// SpecExtractedInfo contains useful data extracted from a valid spec
type SpecExtractedInfo struct {
	Operations  []string                   `json:"operations"`
	AuthSchemes []SpecExtractedAuthScheme   `json:"auth_schemes"`
	ServerURL   string                     `json:"server_url"`
}

// SpecExtractedAuthScheme describes an auth scheme found in the spec
type SpecExtractedAuthScheme struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	In      string `json:"in,omitempty"`
	KeyName string `json:"key_name,omitempty"`
}

// DatasourceTestResult contains the result of datasource connectivity testing
type DatasourceTestResult struct {
	EmbedderValid   bool   `json:"embedder_valid"`
	EmbedderError   string `json:"embedder_error,omitempty"`
	EmbedderVendor  string `json:"embedder_vendor"`
	EmbedderModel   string `json:"embedder_model"`
	EmbedTestPassed bool   `json:"embed_test_passed"`
	EmbedTestError  string `json:"embed_test_error,omitempty"`
}

// ValidateOASSpec validates an OpenAPI spec and returns structured results
func (s *Service) ValidateOASSpec(oasSpecBase64 string) (*SpecValidationResult, error) {
	result := &SpecValidationResult{
		Valid:    true,
		Errors:   []SpecValidationError{},
		Warnings: []SpecValidationWarning{},
	}

	// Step 1: Decode base64
	specBytes, err := base64.StdEncoding.DecodeString(oasSpecBase64)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, SpecValidationError{
			Field:   "oas_spec",
			Message: fmt.Sprintf("Failed to decode base64: %v", err),
		})
		return result, nil
	}

	// Step 2: Check spec size (1MB limit)
	if len(specBytes) > 1024*1024 {
		result.Valid = false
		result.Errors = append(result.Errors, SpecValidationError{
			Field:   "oas_spec",
			Message: fmt.Sprintf("Spec size (%d bytes) exceeds maximum of 1MB", len(specBytes)),
		})
		return result, nil
	}

	// Step 3: Attempt full universalclient validation (this catches version, servers, operationIDs, auth schemes)
	client, err := universalclient.NewClient(specBytes, "")
	if err != nil {
		result.Valid = false

		// Parse the error to provide structured feedback
		errMsg := err.Error()

		switch {
		case strings.Contains(errMsg, "unsupported OpenAPI version"):
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "openapi",
				Message: errMsg,
			})
		case strings.Contains(errMsg, "servers entry"):
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "servers",
				Message: errMsg,
			})
		case strings.Contains(errMsg, "operationID"):
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "paths",
				Message: errMsg,
			})
		case strings.Contains(errMsg, "authentication type"):
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "components.securitySchemes",
				Message: errMsg,
			})
		case strings.Contains(errMsg, "failed to build V3 model"):
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "oas_spec",
				Message: fmt.Sprintf("Failed to parse OpenAPI spec: %v", errMsg),
			})
		default:
			result.Errors = append(result.Errors, SpecValidationError{
				Field:   "oas_spec",
				Message: errMsg,
			})
		}
		return result, nil
	}

	// Step 4: Extract useful information from the valid spec
	extracted := &SpecExtractedInfo{}

	// Extract operations
	operations, err := client.ListOperations()
	if err == nil {
		extracted.Operations = operations
	}

	// Extract auth schemes
	authSchemes := client.GetSupportedAuthSchemes()
	for _, scheme := range authSchemes {
		extracted.AuthSchemes = append(extracted.AuthSchemes, SpecExtractedAuthScheme{
			Type:    scheme.Type,
			Name:    scheme.Name,
			In:      scheme.In,
			KeyName: scheme.KeyName,
		})
	}

	result.Extracted = extracted

	// Step 5: Add warnings for quality issues
	if len(operations) == 0 {
		result.Warnings = append(result.Warnings, SpecValidationWarning{
			Field:   "paths",
			Message: "No operations found in the spec",
		})
	}

	if len(authSchemes) == 0 {
		result.Warnings = append(result.Warnings, SpecValidationWarning{
			Field:   "components.securitySchemes",
			Message: "No authentication schemes defined — the API will be called without authentication",
		})
	}

	return result, nil
}

// isPrivateIP checks if an IP address is in a private/reserved range
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16", "::1/128", "fc00::/7",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// validateEmbedURLForSSRF checks that the embed URL does not target internal network addresses.
// Skipped if ALLOW_INTERNAL_NETWORK_ACCESS is set to "true".
func validateEmbedURLForSSRF(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	if os.Getenv("ALLOW_INTERNAL_NETWORK_ACCESS") == "true" {
		return nil
	}

	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid embed URL: %w", err)
	}

	hostname := parsed.Hostname()
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve embed URL hostname %q: %w", hostname, err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("embed URL %q resolves to private/internal IP %s — blocked for security", rawURL, ip)
		}
	}
	return nil
}

// TestDatasourceConnectivity tests that an embedder can be created and optionally performs a test embedding
func (s *Service) TestDatasourceConnectivity(embedVendor, embedURL, embedAPIKey, embedModel string) (*DatasourceTestResult, error) {
	result := &DatasourceTestResult{
		EmbedderVendor: embedVendor,
		EmbedderModel:  embedModel,
	}

	// Validate URL against SSRF before making any outbound requests
	if err := validateEmbedURLForSSRF(embedURL); err != nil {
		result.EmbedderValid = false
		result.EmbedderError = err.Error()
		return result, nil
	}

	// Build a temporary datasource model for the switches package
	ds := &models.Datasource{
		EmbedVendor: models.Vendor(embedVendor),
		EmbedUrl:    embedURL,
		EmbedAPIKey: embedAPIKey,
		EmbedModel:  embedModel,
	}

	// Step 1: Validate that the embedder can be created
	embedder, err := switches.GetEmbedder(ds)
	if err != nil {
		result.EmbedderValid = false
		result.EmbedderError = err.Error()
		return result, nil
	}
	result.EmbedderValid = true

	// Step 2: Attempt a test embedding to verify actual connectivity (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testTexts := []string{"connectivity test"}
	_, err = embedder.EmbedDocuments(ctx, testTexts)
	if err != nil {
		result.EmbedTestPassed = false
		result.EmbedTestError = fmt.Sprintf("Embedding test failed: %v", err)
		return result, nil
	}
	result.EmbedTestPassed = true

	return result, nil
}
