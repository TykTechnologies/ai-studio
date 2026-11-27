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
var ErrInvalidEndpoint = errors.New("vertex API endpoint must be project:location (for example my-project:us-central1) or an https URL, optionally ending in /v1/projects/{project}/locations/{location}")

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
	if !projectPattern.MatchString(project) || !locationPattern.MatchString(location) {
		return "", "", false
	}
	return project, location, true
}

// projectPattern is a project ID or number (lowercase letters, digits and
// hyphens), optionally after one domain prefix: a domain-scoped project
// (example.com:my-project). Nothing else may carry a colon, so a:b:location
// (a not a domain) is refused rather than read as a project. It becomes part
// of the upstream path.
var projectPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)+:)?[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// apiVersionPattern is a Vertex API version segment (v1, v1beta1).
var apiVersionPattern = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]*)?$`)

// ParseURLEndpoint splits a URL endpoint into the base the proxy sends to
// and, when its path ends in [/v1]/projects/{project}/locations/{location}
// (a trailing slash allowed), that project and location:
//
//	https://vertex.example/v1/projects/p/locations/us-central1 -> https://vertex.example, p, us-central1
//	https://gw.example/vertex/projects/p/locations/eu          -> https://gw.example/vertex, p, eu
//	https://vertex.example                                     -> https://vertex.example, "", ""
//
// Whatever precedes the version (or projects) segment is a path prefix and
// stays in the base. A path with a projects segment of any other shape, or
// with an invalid project or location, is ErrInvalidEndpoint.
func ParseURLEndpoint(endpoint string) (base, project, location string, err error) {
	u, perr := url.Parse(strings.TrimSpace(endpoint))
	if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", "", ErrInvalidEndpoint
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	at := -1
	for i, seg := range segs {
		if seg == "projects" {
			at = i
			break
		}
	}
	if at < 0 {
		return strings.TrimSpace(endpoint), "", "", nil
	}
	if len(segs) != at+4 || segs[at+2] != "locations" ||
		!projectPattern.MatchString(segs[at+1]) || !locationPattern.MatchString(segs[at+3]) {
		return "", "", "", ErrInvalidEndpoint
	}
	prefix := segs[:at]
	if len(prefix) > 0 && apiVersionPattern.MatchString(prefix[len(prefix)-1]) {
		prefix = prefix[:len(prefix)-1]
	}
	base = u.Scheme + "://" + u.Host
	if len(prefix) > 0 {
		base += "/" + strings.Join(prefix, "/")
	}
	return base, segs[at+1], segs[at+3], nil
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
// endpoint, a test server) is used as it is, except that a trailing
// [/v1]/projects/{p}/locations/{l} in its path (see ParseURLEndpoint) names
// the project and location and is not part of the base. The path the caller
// sends (/v1/projects/{p}/locations/{l}/publishers/google/models/{m}:generateContent)
// is appended to the base unchanged.
func UpstreamBaseURL(llm *models.LLM) (string, error) {
	return UpstreamBaseURLFor(llm.APIEndpoint)
}

// UpstreamBaseURLFor is UpstreamBaseURL for an endpoint value.
func UpstreamBaseURLFor(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if strings.Contains(endpoint, "://") {
		base, _, _, err := ParseURLEndpoint(endpoint)
		return base, err
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

// ErrNoProjectLocation is returned when a Gemini API path cannot be mapped to
// a Vertex path because the LLM's endpoint is a URL, which names neither the
// project nor the location.
var ErrNoProjectLocation = errors.New("vertex LLM endpoint is a URL, so a Gemini API path cannot be mapped to a Vertex path; set the endpoint to project:location or call the Vertex path (/v1/projects/{project}/locations/{location}/publishers/google/models/{model}:{method})")

// geminiAPIPath matches a Gemini API (generativelanguage) model call:
// /v1beta/models/{model}:{method} or /v1/models/{model}:{method}.
var geminiAPIPath = regexp.MustCompile(`^/(v1|v1beta)/models/([^/:]+):([A-Za-z]+)$`)

// NativePathFromGeminiAPI maps a Gemini API path (what the Google AI client
// sends) to the Vertex path for the LLM's project and location:
//
//	/v1beta/models/{m}:{method} -> /v1/projects/{p}/locations/{l}/publishers/google/models/{m}:{method}
//
// The query (alt=sse) is not part of path and stays as it is. changed is
// false, with no error, for any other path (a Vertex path included), which is
// left to the upstream as it is.
func NativePathFromGeminiAPI(llm *models.LLM, path string) (native string, changed bool, err error) {
	m := geminiAPIPath.FindStringSubmatch(path)
	if m == nil {
		return path, false, nil
	}
	project, location, ok := ParseProjectLocation(llm.APIEndpoint)
	if !ok {
		return path, false, ErrNoProjectLocation
	}
	return fmt.Sprintf("/v1/projects/%s/locations/%s/publishers/google/models/%s:%s", project, location, m[2], m[3]), true, nil
}
