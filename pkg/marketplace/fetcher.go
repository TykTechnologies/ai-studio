package marketplace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/netguard"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// ErrFetchFailed is the one error for a marketplace URL that could not be
// fetched, whatever the cause: the address was refused (an internal address,
// a name that resolves to one or does not resolve, a redirect to one, a host
// outside LLM_UPSTREAM_ALLOWED_HOSTS), nothing answered, the connection
// failed, the answer was not 200, or it was too large. The cause is logged
// and never returned: the error reaches API callers (validate) and stored
// sync status, and a different message per cause tells a caller what
// listens on which address inside the network.
var ErrFetchFailed = errors.New("the marketplace URL could not be fetched: the address was refused, did not answer, or did not return the file. Internal addresses are refused unless the host is in LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS or ALLOW_INTERNAL_NETWORK_ACCESS=true. The cause is in the AI Studio log")

// ErrInvalidDocument is the error for an answer that is not the YAML
// document asked for. The parser's message quotes the answer, so it is
// logged and not returned.
var ErrInvalidDocument = errors.New("the marketplace URL did not return a valid marketplace document (YAML)")

const (
	// maxIndexBytes and maxManifestBytes cap what is read of an answer.
	maxIndexBytes    = 32 << 20
	maxManifestBytes = 1 << 20
)

// CheckIndexURL applies, without fetching, the rule every marketplace fetch
// is under: an http or https URL whose host is not an internal address, does
// not resolve to one, and resolves (netguard's plugin host rule, the one
// plugin registries are under). For a URL that is stored now and fetched
// later (adding a marketplace). The fetch itself is checked again, on the
// address it connects to. The error is ErrFetchFailed.
func CheckIndexURL(indexURL string) error {
	u, err := url.Parse(strings.TrimSpace(indexURL))
	if err == nil && u.Hostname() == "" {
		err = errors.New("no host")
	}
	if err == nil {
		err = netguard.ValidateUpstreamURL(u)
	}
	if err == nil {
		err = netguard.CheckPluginHost(u.Hostname())
	}
	if err != nil {
		log.Warn().Err(err).Str("url", redactURL(indexURL)).Msg("Marketplace URL refused")
		return ErrFetchFailed
	}
	return nil
}

// redactURL is rawURL without userinfo, for logs.
func redactURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "(unparseable URL)"
	}
	return u.Redacted()
}

// Fetcher handles fetching marketplace indexes and manifests
type Fetcher struct {
	httpClient *http.Client
	userAgent  string
}

// NewFetcher creates a new marketplace fetcher
func NewFetcher(timeout time.Duration) *Fetcher {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Fetcher{
		// The transport plugin registries use: every dial, redirects
		// included, is checked on the address it connects to, so a name
		// that resolves to an internal address, or a public URL that
		// redirects to one, is refused like the address itself.
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: netguard.PluginHTTPTransport(),
		},
		userAgent: "TykAIStudio/1.0 Marketplace-Fetcher",
	}
}

// get fetches fetchURL and returns its body (at most maxBytes) and the
// response, whose body is already closed. Every failure is ErrFetchFailed,
// with the cause in the log. notModified is true for a 304.
func (f *Fetcher) get(ctx context.Context, fetchURL string, header http.Header, maxBytes int64) (body []byte, resp *http.Response, notModified bool, err error) {
	fail := func(cause error) ([]byte, *http.Response, bool, error) {
		log.Warn().Err(cause).Str("url", redactURL(fetchURL)).Msg("Marketplace fetch failed")
		return nil, nil, false, ErrFetchFailed
	}
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return fail(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/x-yaml, text/yaml, application/yaml")

	resp, err = f.httpClient.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && (header.Get("If-None-Match") != "" || header.Get("If-Modified-Since") != "") {
		return nil, resp, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return fail(fmt.Errorf("unexpected status code: %d", resp.StatusCode))
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return fail(fmt.Errorf("failed to read response body: %w", err))
	}
	if int64(len(body)) > maxBytes {
		return fail(fmt.Errorf("response is larger than %d bytes", maxBytes))
	}
	return body, resp, false, nil
}

// parse unmarshals a fetched YAML document; a failure is ErrInvalidDocument.
func parse(fetchURL string, body []byte, out any) error {
	if err := yaml.Unmarshal(body, out); err != nil {
		log.Warn().Err(err).Str("url", redactURL(fetchURL)).Msg("Marketplace document is not valid YAML")
		return ErrInvalidDocument
	}
	return nil
}

// FetchIndex fetches and parses the marketplace index.yaml
func (f *Fetcher) FetchIndex(ctx context.Context, indexURL string) (*MarketplaceIndex, *FetchMetadata, error) {
	log.Info().Str("url", indexURL).Msg("Fetching marketplace index")

	body, resp, _, err := f.get(ctx, indexURL, nil, maxIndexBytes)
	if err != nil {
		return nil, nil, err
	}

	// Parse YAML
	var index MarketplaceIndex
	if err := parse(indexURL, body, &index); err != nil {
		return nil, nil, err
	}

	// Extract metadata from HTTP headers
	metadata := &FetchMetadata{
		ETag:         resp.Header.Get("ETag"),
		LastModified: parseHTTPDate(resp.Header.Get("Last-Modified")),
		ContentLength: resp.ContentLength,
		FetchedAt:    time.Now(),
	}

	log.Info().
		Str("url", indexURL).
		Int("plugin_count", countPlugins(&index)).
		Msg("Successfully fetched marketplace index")

	return &index, metadata, nil
}

// FetchManifest fetches and parses a plugin manifest.yaml
func (f *Fetcher) FetchManifest(ctx context.Context, manifestURL string) (*PluginManifest, error) {
	log.Debug().Str("url", manifestURL).Msg("Fetching plugin manifest")

	body, _, _, err := f.get(ctx, manifestURL, nil, maxManifestBytes)
	if err != nil {
		return nil, err
	}

	// Parse YAML
	var manifest PluginManifest
	if err := parse(manifestURL, body, &manifest); err != nil {
		return nil, err
	}

	log.Debug().
		Str("url", manifestURL).
		Str("plugin_id", manifest.ID).
		Str("version", manifest.Version).
		Msg("Successfully fetched plugin manifest")

	return &manifest, nil
}

// FetchIndexConditional fetches the index only if it has been modified.
// When forceRefresh is true, a cache-busting query parameter is appended and
// conditional headers are skipped to bypass CDN caches (e.g. GitHub's).
func (f *Fetcher) FetchIndexConditional(ctx context.Context, indexURL, etag string, lastModified time.Time, forceRefresh bool) (*MarketplaceIndex, *FetchMetadata, bool, error) {
	fetchURL := indexURL
	if forceRefresh {
		u, err := url.Parse(indexURL)
		if err != nil {
			log.Warn().Err(err).Msg("Marketplace index URL does not parse")
			return nil, nil, false, ErrFetchFailed
		}
		q := u.Query()
		q.Set("_cb", strconv.FormatInt(time.Now().UnixNano(), 10))
		u.RawQuery = q.Encode()
		fetchURL = u.String()
	}

	// Add conditional headers (skipped on force-refresh to bypass CDN caches)
	header := http.Header{}
	if !forceRefresh {
		if etag != "" {
			header.Set("If-None-Match", etag)
		}
		if !lastModified.IsZero() {
			header.Set("If-Modified-Since", lastModified.Format(http.TimeFormat))
		}
	}

	body, resp, notModified, err := f.get(ctx, fetchURL, header, maxIndexBytes)
	if err != nil {
		return nil, nil, false, err
	}

	// Not modified
	if notModified {
		log.Debug().Str("url", indexURL).Msg("Marketplace index not modified")
		return nil, nil, false, nil
	}

	var index MarketplaceIndex
	if err := parse(indexURL, body, &index); err != nil {
		return nil, nil, false, err
	}

	metadata := &FetchMetadata{
		ETag:         resp.Header.Get("ETag"),
		LastModified: parseHTTPDate(resp.Header.Get("Last-Modified")),
		ContentLength: resp.ContentLength,
		FetchedAt:    time.Now(),
	}

	log.Info().
		Str("url", indexURL).
		Int("plugin_count", countPlugins(&index)).
		Msg("Marketplace index updated")

	return &index, metadata, true, nil
}

// FetchMetadata contains HTTP response metadata
type FetchMetadata struct {
	ETag          string
	LastModified  time.Time
	ContentLength int64
	FetchedAt     time.Time
}

// parseHTTPDate parses HTTP date header
func parseHTTPDate(dateStr string) time.Time {
	if dateStr == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(dateStr)
	if err != nil {
		log.Warn().Str("date", dateStr).Err(err).Msg("Failed to parse HTTP date")
		return time.Time{}
	}
	return t
}

// countPlugins counts total plugins in index
func countPlugins(index *MarketplaceIndex) int {
	count := 0
	for _, versions := range index.Plugins {
		count += len(versions)
	}
	return count
}

// ValidateIndex performs basic validation on the marketplace index
func ValidateIndex(index *MarketplaceIndex) error {
	if index == nil {
		return fmt.Errorf("index is nil")
	}

	if index.APIVersion == "" {
		return fmt.Errorf("index missing apiVersion")
	}

	if index.Plugins == nil {
		return fmt.Errorf("index missing plugins map")
	}

	// Validate each plugin entry
	for pluginID, versions := range index.Plugins {
		if len(versions) == 0 {
			return fmt.Errorf("plugin %s has no versions", pluginID)
		}

		for i, plugin := range versions {
			if plugin.ID != pluginID {
				return fmt.Errorf("plugin %s version %d: ID mismatch (expected %s, got %s)",
					pluginID, i, pluginID, plugin.ID)
			}
			if plugin.Version == "" {
				return fmt.Errorf("plugin %s version %d: missing version", pluginID, i)
			}
			if plugin.OCIRegistry == "" || plugin.OCIRepository == "" {
				return fmt.Errorf("plugin %s version %s: incomplete OCI reference", pluginID, plugin.Version)
			}
		}
	}

	return nil
}

// ValidateManifest performs basic validation on a plugin manifest
func ValidateManifest(manifest *PluginManifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}

	if manifest.ID == "" {
		return fmt.Errorf("manifest missing id")
	}
	if manifest.Name == "" {
		return fmt.Errorf("manifest missing name")
	}
	if manifest.Version == "" {
		return fmt.Errorf("manifest missing version")
	}

	// Validate OCI info
	if manifest.OCI.Registry == "" {
		return fmt.Errorf("manifest missing OCI registry")
	}
	if manifest.OCI.Repository == "" {
		return fmt.Errorf("manifest missing OCI repository")
	}
	if manifest.OCI.Tag == "" && manifest.OCI.Digest == "" {
		return fmt.Errorf("manifest missing OCI tag or digest")
	}

	// Validate capabilities
	if len(manifest.Capabilities.Hooks) == 0 {
		return fmt.Errorf("manifest missing capabilities.hooks")
	}

	return nil
}
