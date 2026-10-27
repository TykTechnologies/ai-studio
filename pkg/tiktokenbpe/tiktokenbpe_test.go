package tiktokenbpe

import (
	"crypto/fips140"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkoukk/tiktoken-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// "a" -> 0, "b" -> 1, "ab" -> 2
const ranks = "YQ== 0\nYg== 1\nYWI= 2\n"

func wantRanks(t *testing.T, got map[string]int) {
	t.Helper()
	assert.Equal(t, map[string]int{"a": 0, "b": 1, "ab": 2}, got)
}

func sha256Name(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// The precomputed names are the cache names tiktoken-go's own loader uses:
// seeded under them, its loader reads the tiny rank table instead of
// downloading the real one.
func TestLegacyCacheNamesMatchTiktoken(t *testing.T) {
	if fips140.Enforced() {
		t.Skip("tiktoken-go's loader computes SHA-1, which fips140=only refuses")
	}
	dir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", dir)
	for url, name := range legacyCacheNames {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(ranks), 0o644))
		got, err := tiktoken.NewDefaultBpeLoader().LoadTiktokenBpe(url)
		require.NoError(t, err, url)
		wantRanks(t, got)
	}
}

func TestLoader_DownloadsAndCachesUnderSHA256Name(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", dir)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(ranks))
	}))
	defer srv.Close()
	url := srv.URL + "/test.tiktoken"

	got, err := Loader{}.LoadTiktokenBpe(url)
	require.NoError(t, err)
	wantRanks(t, got)
	cached, err := os.ReadFile(filepath.Join(dir, sha256Name(url)))
	require.NoError(t, err)
	assert.Equal(t, ranks, string(cached))

	got, err = Loader{}.LoadTiktokenBpe(url)
	require.NoError(t, err)
	wantRanks(t, got)
	assert.Equal(t, 1, hits, "the second load reads the cache")
}

// A cache pre-seeded under tiktoken-go's SHA-1 name (an air-gapped install)
// is still read, with no download.
func TestLoader_ReadsLegacyCacheName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", dir)
	url := "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"
	require.NoError(t, os.WriteFile(filepath.Join(dir, legacyCacheNames[url]), []byte(ranks), 0o644))

	got, err := Loader{}.LoadTiktokenBpe(url)
	require.NoError(t, err)
	wantRanks(t, got)
}

func TestLoader_LocalFileAndErrors(t *testing.T) {
	t.Setenv("TIKTOKEN_CACHE_DIR", t.TempDir())
	f := filepath.Join(t.TempDir(), "ranks.tiktoken")
	require.NoError(t, os.WriteFile(f, []byte(ranks), 0o644))
	got, err := Loader{}.LoadTiktokenBpe(f)
	require.NoError(t, err)
	wantRanks(t, got)

	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err = Loader{}.LoadTiktokenBpe(srv.URL + "/missing.tiktoken")
	assert.Error(t, err, "a failed download is an error, not an empty rank table")

	bad := filepath.Join(t.TempDir(), "bad.tiktoken")
	require.NoError(t, os.WriteFile(bad, []byte("not-a-rank-line\n"), 0o644))
	_, err = Loader{}.LoadTiktokenBpe(bad)
	assert.Error(t, err)
}
