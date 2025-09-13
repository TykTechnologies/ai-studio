// pkg/ociplugins/parser.go
package ociplugins

import (
	"net/url"
	"regexp"
	"strings"
)

// ociRefRegex matches OCI references with optional digest or tag
// Format: oci://registry/repository[@digest|:tag][?params]
var ociRefRegex = regexp.MustCompile(`^oci://([^/]+)/([^@:?]+)(?:@([^?]+)|:([^?]+))?(?:\?(.+))?$`)

// ParseOCICommand parses an OCI command string and returns the reference and parameters
func ParseOCICommand(command string) (*OCIReference, *OCIPluginParams, error) {
	if !strings.HasPrefix(command, "oci://") {
		return nil, nil, &ErrInvalidOCIReference{
			Reference: command,
			Reason:    "must start with 'oci://'",
		}
	}

	matches := ociRefRegex.FindStringSubmatch(command)
	if len(matches) != 6 {
		return nil, nil, &ErrInvalidOCIReference{
			Reference: command,
			Reason:    "invalid format, expected oci://registry/repository[@digest|:tag][?params]",
		}
	}

	registry := matches[1]
	repository := matches[2]
	digest := matches[3]
	tag := matches[4]
	queryString := matches[5]

	// Validate that we have either digest or tag (prefer digest)
	if digest == "" && tag == "" {
		tag = "latest" // Default to latest if neither specified
	}

	// Parse query parameters
	params := make(map[string]string)
	if queryString != "" {
		values, err := url.ParseQuery(queryString)
		if err != nil {
			return nil, nil, &ErrInvalidOCIReference{
				Reference: command,
				Reason:    "invalid query parameters: " + err.Error(),
			}
		}

		for key, vals := range values {
			if len(vals) > 0 {
				params[key] = vals[0] // Take first value if multiple
			}
		}
	}

	// Create reference
	ref := &OCIReference{
		Registry:   registry,
		Repository: repository,
		Digest:     digest,
		Tag:        tag,
		Params:     params,
	}

	// Parse plugin-specific parameters
	pluginParams := &OCIPluginParams{
		Architecture: getParamWithDefault(params, "arch", "linux/amd64"),
		PublicKey:    params["pubkey"],
		AuthConfig:   params["auth"],
	}

	// Validate architecture format
	if !isValidArchitecture(pluginParams.Architecture) {
		return nil, nil, &ErrInvalidOCIReference{
			Reference: command,
			Reason:    "invalid architecture format: " + pluginParams.Architecture,
		}
	}

	return ref, pluginParams, nil
}

// getParamWithDefault returns parameter value or default if not present
func getParamWithDefault(params map[string]string, key, defaultValue string) string {
	if value, exists := params[key]; exists && value != "" {
		return value
	}
	return defaultValue
}

// isValidArchitecture checks if architecture follows OS/ARCH format
func isValidArchitecture(arch string) bool {
	parts := strings.Split(arch, "/")
	if len(parts) != 2 {
		return false
	}

	validOS := map[string]bool{
		"linux":   true,
		"darwin":  true,
		"windows": true,
	}

	validArch := map[string]bool{
		"amd64": true,
		"arm64": true,
		"386":   true,
		"arm":   true,
	}

	return validOS[parts[0]] && validArch[parts[1]]
}

// ValidateOCIReference performs additional validation on an OCI reference
func ValidateOCIReference(ref *OCIReference, config *OCIConfig) error {
	// Check if registry is allowed
	if len(config.AllowedRegistries) > 0 {
		allowed := false
		for _, allowedRegistry := range config.AllowedRegistries {
			if ref.Registry == allowedRegistry {
				allowed = true
				break
			}
		}
		if !allowed {
			return &ErrRegistryNotAllowed{
				Registry:        ref.Registry,
				AllowedRegistries: config.AllowedRegistries,
			}
		}
	}

	// Validate that we have a digest for production use
	if ref.Digest == "" {
		// Using tags is less secure but might be acceptable in development
		// We could make this configurable
	}

	return nil
}

// NormalizeOCIReference ensures consistent formatting of OCI references
func NormalizeOCIReference(ref *OCIReference) *OCIReference {
	normalized := &OCIReference{
		Registry:   strings.ToLower(ref.Registry),
		Repository: strings.ToLower(ref.Repository),
		Digest:     ref.Digest,
		Tag:        ref.Tag,
		Params:     make(map[string]string),
	}

	// Copy and normalize params
	for k, v := range ref.Params {
		normalized.Params[strings.ToLower(k)] = v
	}

	return normalized
}