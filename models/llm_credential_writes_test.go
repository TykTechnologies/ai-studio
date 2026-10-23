package models

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// LLM credentials are encrypted at rest by the LLM's save hooks, which a
// column or map update (Update("metadata", ...), UpdateColumns(map{...}))
// never runs: such a write would store a credential in plaintext. This test
// fails on any new one in Studio's code. The writes that encrypt for
// themselves are listed in allowed, by file and a fragment of the line.
func TestLLMCredentialColumnsAreOnlySavedWhole(t *testing.T) {
	allowed := map[string]string{
		"services/llm_service.go":   "EncryptLLMMetadata(",         // UpdateLLMMetadata
		"models/llm_credentials.go": "q.UpdateColumns(map[string]", // the backfill
	}
	columnUpdate := regexp.MustCompile(`\.(Update|UpdateColumn)\(\s*"(api_key|metadata)"`)
	mapUpdate := regexp.MustCompile(`\.(Updates|UpdateColumns)\(.*"(api_key|metadata)"\s*:`)
	mentionsLLM := regexp.MustCompile(`LLM|llm`)

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// A directory the test cannot read (a container's cache) holds
			// no Studio source.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			switch rel {
			case "third_party", "microgateway", "community", "node_modules", "ui", "docs", "data", "oci-cache":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		var window []string // this line and the three before it: the statement
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			window = append(window, line)
			if len(window) > 4 {
				window = window[1:]
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if !columnUpdate.MatchString(line) && !mapUpdate.MatchString(line) {
				continue
			}
			if !mentionsLLM.MatchString(strings.Join(window, "\n")) {
				continue
			}
			if frag, ok := allowed[filepath.ToSlash(rel)]; ok && strings.Contains(line, frag) {
				continue
			}
			found = append(found, filepath.ToSlash(rel)+":"+strconv.Itoa(n)+": "+trimmed)
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("LLM credential columns written without the LLM's save hooks (they would be stored in plaintext); "+
			"save the LLM whole, or encrypt with EncryptLLMMetadata and list the write here:\n%s", strings.Join(found, "\n"))
	}
}
