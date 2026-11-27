package vertexVendor

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// ErrInvalidEndpoint is returned when a Vertex LLM's API endpoint is neither
// "project:location" nor an http(s) URL.
var ErrInvalidEndpoint = errors.New("vertex API endpoint must be project:location (for example my-project:us-central1) or an https URL")

// locationPattern is a Google Cloud location ID: a region (us-central1), a
// multi-region (us, eu) or global. It becomes part of a host name, so nothing
// else is accepted.
var locationPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// multiRegions are the Vertex multi-regional locations, served on their own
// regional-endpoint hosts (aiplatform.{loc}.rep.googleapis.com).
var multiRegions = map[string]bool{"us": true, "eu": true}

// ParseProjectLocation splits a "project:location" endpoint. The location is
// after the last colon, so a domain-scoped project ID (example.com:my-project)
// keeps its own colon. ok is false for a URL or a malformed value.
func ParseProjectLocation(endpoint string) (project, location string, ok bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.Contains(endpoint, "/") {
		return "", "", false
	}
	i := strings.LastIndex(endpoint, ":")
	if i <= 0 || i == len(endpoint)-1 {
		return "", "", false
	}
	project, location = endpoint[:i], endpoint[i+1:]
	if !locationPattern.MatchString(location) {
		return "", "", false
	}
	return project, location, true
}

// UpstreamBaseURL is the base URL the proxy forwards a Vertex LLM's native
// traffic to. A "project:location" endpoint maps to the location's service
// endpoint:
//
//   - a region (us-central1): https://us-central1-aiplatform.googleapis.com
//   - a multi-region (us, eu): https://aiplatform.us.rep.googleapis.com
//   - global: https://aiplatform.googleapis.com
//
// An endpoint that is already an http(s) URL (a private service connect
// endpoint, a test server) is returned as it is. The path the caller sends
// (/v1/projects/{p}/locations/{l}/publishers/google/models/{m}:generateContent)
// is appended to it unchanged.
func UpstreamBaseURL(llm *models.LLM) (string, error) {
	return UpstreamBaseURLFor(llm.APIEndpoint)
}

// UpstreamBaseURLFor is UpstreamBaseURL for an endpoint value.
func UpstreamBaseURLFor(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return "", ErrInvalidEndpoint
		}
		return endpoint, nil
	}
	_, location, ok := ParseProjectLocation(endpoint)
	if !ok {
		return "", ErrInvalidEndpoint
	}
	switch {
	case location == "global":
		return "https://aiplatform.googleapis.com", nil
	case multiRegions[location]:
		return fmt.Sprintf("https://aiplatform.%s.rep.googleapis.com", location), nil
	default:
		return fmt.Sprintf("https://%s-aiplatform.googleapis.com", location), nil
	}
}
