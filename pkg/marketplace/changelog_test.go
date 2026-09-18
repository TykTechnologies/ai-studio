package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChangelogURL(t *testing.T) {
	const index = "https://raw.githubusercontent.com/acme/plugins/main/index.yaml"

	t.Run("sits next to the version's manifest", func(t *testing.T) {
		got, err := ChangelogURL("https://raw.githubusercontent.com/acme/plugins/main/plugins/llm-cache/1.1.0/manifest.yaml?x=1#frag", index)
		require.NoError(t, err)
		assert.Equal(t, "https://raw.githubusercontent.com/acme/plugins/main/plugins/llm-cache/1.1.0/CHANGELOG.md", got)
	})

	t.Run("refuses a manifest on another host", func(t *testing.T) {
		// A crafted index must not be able to make Studio fetch internal URLs.
		for _, manifest := range []string{
			"https://169.254.169.254/latest/meta-data/manifest.yaml",
			"http://raw.githubusercontent.com/acme/plugins/main/plugins/x/1.0.0/manifest.yaml", // scheme downgrade
			"https://raw.githubusercontent.com.evil.example/x/manifest.yaml",
			"file:///etc/manifest.yaml",
			"",
		} {
			_, err := ChangelogURL(manifest, index)
			assert.Error(t, err, manifest)
		}
	})
}

func TestFetchChangelog(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("internal secret"))
	}))
	defer other.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok/CHANGELOG.md":
			w.Write([]byte("# Changelog\n\n## [1.1.0]\n- Fixed things\n"))
		case "/big/CHANGELOG.md":
			w.Write([]byte(strings.Repeat("a", maxChangelogBytes+1000)))
		case "/redirect/CHANGELOG.md":
			http.Redirect(w, r, other.URL+"/secret", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	f := NewFetcher(5 * time.Second)

	body, err := f.FetchChangelog(context.Background(), server.URL+"/ok/CHANGELOG.md")
	require.NoError(t, err)
	assert.Contains(t, body, "Fixed things")

	body, err = f.FetchChangelog(context.Background(), server.URL+"/big/CHANGELOG.md")
	require.NoError(t, err)
	assert.Len(t, body, maxChangelogBytes, "the body is capped")

	_, err = f.FetchChangelog(context.Background(), server.URL+"/missing/CHANGELOG.md")
	assert.Error(t, err)

	body, err = f.FetchChangelog(context.Background(), server.URL+"/redirect/CHANGELOG.md")
	assert.Error(t, err, "a redirect to another host is not followed")
	assert.NotContains(t, body, "internal secret")
}
