package secrets

import (
	"regexp"
	"strings"
	"testing"
)

// encToken matches a value EncryptValue wrote: "$ENC/" and a legacy
// (base64url) or v2 ("v2:" + base64url) ciphertext. A v3 value
// ("$ENC/v3:<iter>:...") is excluded by the caller: the pattern stops at
// "v3" and the next character is ':'.
var encToken = regexp.MustCompile(`\$ENC/(?:v2:)?[A-Za-z0-9_\-]+=*`)

// WritesFIPSFormat reports whether this process writes the FIPS 140-3
// formats (v3 secrets): crypto/fips140.Enabled.
func WritesFIPSFormat() bool { return fipsEnabled() }

// ReencryptEncValues re-encrypts, in the format this process writes, every
// legacy or v2 "$ENC/" value in text: a column holding one EncryptValue
// output, or a JSON document holding several (LLM credential metadata, a
// submission's payload). v3 values are left alone, and so is a token that
// does not decrypt with the configured key (it is not ours, or was written
// under another key). It reads legacy AES-CFB values outside FIPS 140-only
// enforcement (decryptForMigration): it is a migration. It returns the new
// text and how many values it re-encrypted; with no key configured it
// changes nothing.
func ReencryptEncValues(text string) (string, int) {
	key := encryptionKey()
	if key == "" || !strings.Contains(text, "$ENC/") {
		return text, 0
	}
	var b strings.Builder
	last, n := 0, 0
	for _, loc := range encToken.FindAllStringIndex(text, -1) {
		start, end := loc[0], loc[1]
		if end < len(text) && text[end] == ':' {
			continue // "$ENC/v3:..." (or not a value we wrote)
		}
		token := text[start:end]
		plain, err := decryptForMigration(key, strings.TrimPrefix(token, "$ENC/"))
		if err != nil {
			continue
		}
		enc, err := encrypt(key, plain)
		if err != nil {
			continue
		}
		b.WriteString(text[last:start])
		b.WriteString("$ENC/" + enc)
		last = end
		n++
	}
	if n == 0 {
		return text, 0
	}
	b.WriteString(text[last:])
	return b.String(), n
}

// ForceFIPSModeForTest makes this package write the FIPS formats (v3) until
// the test ends, so other packages' standard test runs can cover their FIPS
// paths. Not safe for parallel tests.
func ForceFIPSModeForTest(tb testing.TB) {
	tb.Helper()
	prev := fipsEnabled
	fipsEnabled = func() bool { return true }
	tb.Cleanup(func() { fipsEnabled = prev })
}
