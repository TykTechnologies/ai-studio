// Package tiktokenbpe loads tiktoken BPE rank files without SHA-1, so token
// counting works under GODEBUG=fips140=only.
//
// github.com/pkoukk/tiktoken-go names its on-disk cache file after the SHA-1
// of the rank file's URL, and crypto/sha1 panics in FIPS 140-only mode. In
// FIPS mode this package installs a BpeLoader that behaves like tiktoken-go's
// own (the same cache directory variables, download and file format) but
// names new cache files after the SHA-256 of the URL. It still reads a cache
// file stored under the SHA-1 name, such as a pre-seeded TIKTOKEN_CACHE_DIR
// in an air-gapped install, through a table of those names computed ahead of
// time, so no SHA-1 runs. Standard builds keep tiktoken-go's loader.
//
// Import it for its side effect wherever tiktoken encodings are loaded.
package tiktokenbpe

import (
	"crypto/fips140"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// legacyCacheNames maps the rank files tiktoken-go loads to the cache file
// names it gives them: the hex SHA-1 of the URL, computed offline.
var legacyCacheNames = map[string]string{
	"https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken": "9b5ad71b2ce5302211f9c61530b329a4922fc6a4",
	"https://openaipublic.blob.core.windows.net/encodings/p50k_base.tiktoken":   "ec7223a39ce59f226a68acc30dc1af2788490e15",
	"https://openaipublic.blob.core.windows.net/encodings/r50k_base.tiktoken":   "0ea1e91bbb3a60f729a8dc8f777fd2fc07cd8df4",
	"https://openaipublic.blob.core.windows.net/encodings/o200k_base.tiktoken":  "fb374d419588a4632f3f557e76b4b70aebbca790",
}

func init() {
	if fips140.Enabled() {
		tiktoken.SetBpeLoader(Loader{})
	}
}

// Loader is a tiktoken.BpeLoader that never uses SHA-1.
type Loader struct{}

// LoadTiktokenBpe implements tiktoken.BpeLoader.
func (Loader) LoadTiktokenBpe(blobpath string) (map[string]int, error) {
	contents, err := readFileCached(blobpath)
	if err != nil {
		return nil, err
	}
	return parseRanks(contents)
}

// cacheDir is where tiktoken-go caches rank files; "" disables caching.
func cacheDir() string {
	if d := os.Getenv("TIKTOKEN_CACHE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("DATA_GYM_CACHE_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "data-gym-cache")
}

func readFileCached(blobpath string) ([]byte, error) {
	dir := cacheDir()
	if dir == "" {
		return readFile(blobpath)
	}
	sum := sha256.Sum256([]byte(blobpath))
	cachePath := filepath.Join(dir, hex.EncodeToString(sum[:]))
	if b, err := os.ReadFile(cachePath); err == nil {
		return b, nil
	}
	if legacy, ok := legacyCacheNames[blobpath]; ok {
		if b, err := os.ReadFile(filepath.Join(dir, legacy)); err == nil {
			return b, nil
		}
	}

	contents, err := readFile(blobpath)
	if err != nil {
		return nil, err
	}
	// Write through a temporary file, as tiktoken-go does, so a concurrent
	// reader never sees a partial file. A cache write failure is not fatal.
	if err := os.MkdirAll(dir, 0o755); err == nil {
		if f, err := os.CreateTemp(dir, filepath.Base(cachePath)+".*.tmp"); err == nil {
			_, werr := f.Write(contents)
			cerr := f.Close()
			if werr != nil || cerr != nil || os.Rename(f.Name(), cachePath) != nil {
				_ = os.Remove(f.Name())
			}
		}
	}
	return contents, nil
}

func readFile(blobpath string) ([]byte, error) {
	if !strings.HasPrefix(blobpath, "http://") && !strings.HasPrefix(blobpath, "https://") {
		return os.ReadFile(blobpath)
	}
	resp, err := http.Get(blobpath) //nolint:gosec,noctx // tiktoken-go's own loader does the same
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tiktoken: fetching %s: %s", blobpath, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// parseRanks reads "<base64 token> <rank>" lines, as tiktoken-go does.
func parseRanks(contents []byte) (map[string]int, error) {
	ranks := make(map[string]int)
	for _, line := range strings.Split(string(contents), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			return nil, fmt.Errorf("tiktoken: malformed rank line %q", line)
		}
		token, err := base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		ranks[string(token)] = rank
	}
	return ranks, nil
}
