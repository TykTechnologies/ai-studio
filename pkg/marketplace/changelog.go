package marketplace

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// maxChangelogBytes caps how much of a CHANGELOG.md is read.
const maxChangelogBytes = 256 * 1024

// ChangelogURL derives the URL of a version's CHANGELOG.md from its manifest
// URL: the publish tool writes both into the same version directory. The URL
// is only returned when it points at the same host as the marketplace index it
// came from, so a crafted manifest_url cannot make Studio fetch (and show an
// admin) content from some other host, such as an internal service.
func ChangelogURL(manifestURL, indexURL string) (string, error) {
	if manifestURL == "" {
		return "", fmt.Errorf("no manifest URL")
	}
	mu, err := url.Parse(manifestURL)
	if err != nil {
		return "", fmt.Errorf("invalid manifest URL: %w", err)
	}
	iu, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("invalid index URL: %w", err)
	}
	if mu.Scheme != "https" && mu.Scheme != "http" {
		return "", fmt.Errorf("unsupported manifest URL scheme %q", mu.Scheme)
	}
	if mu.Scheme != iu.Scheme || !strings.EqualFold(mu.Host, iu.Host) {
		return "", fmt.Errorf("manifest URL host does not match the marketplace index host")
	}
	mu.Path = path.Join(path.Dir(mu.Path), "CHANGELOG.md")
	mu.RawQuery = ""
	mu.Fragment = ""
	return mu.String(), nil
}

// FetchChangelog fetches a CHANGELOG.md as text, truncated to maxChangelogBytes.
func (f *Fetcher) FetchChangelog(ctx context.Context, changelogURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", changelogURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/markdown, text/plain")

	// Same transport, but a redirect may not leave the host that was vetted.
	client := *f.httpClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !strings.EqualFold(next.URL.Host, req.URL.Host) {
			return http.ErrUseLastResponse
		}
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch changelog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChangelogBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read changelog: %w", err)
	}
	return string(body), nil
}
