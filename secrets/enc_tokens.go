package secrets

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// encValue matches a whole value EncryptValue wrote: "$ENC/" and a legacy
// (base64url) or v2 ("v2:" + base64url) ciphertext, and nothing else, which
// is exactly what DecryptValue decrypts. A v3 value ("$ENC/v3:<iter>:...")
// does not match. Text that merely contains such a token (a description, a
// header value) is not a value we wrote and is never touched: the legacy
// reader is unauthenticated and would turn it into garbage.
var encValue = regexp.MustCompile(`^\$ENC/(?:v2:)?[A-Za-z0-9_\-]+=*$`)

// WritesFIPSFormat reports whether this process writes the FIPS 140-3
// formats (v3 secrets): crypto/fips140.Enabled.
func WritesFIPSFormat() bool { return fipsEnabled() }

// ReencryptEncValue re-encrypts, in the format this process writes, a column
// value that is wholly a legacy or v2 EncryptValue output. Anything else (a
// v3 value, plain text, text containing a token, a value that does not
// decrypt with the configured key) comes back unchanged, with ok false.
// Legacy AES-CFB values are read outside FIPS 140-only enforcement
// (decryptForMigration): this is a migration.
func ReencryptEncValue(value string) (string, bool) {
	key := encryptionKey()
	if key == "" || !encValue.MatchString(value) {
		return value, false
	}
	plain, err := decryptForMigration(key, strings.TrimPrefix(value, "$ENC/"))
	if err != nil {
		return value, false
	}
	enc, err := encrypt(key, plain)
	if err != nil {
		return value, false
	}
	return "$ENC/" + enc, true
}

// ReencryptEncJSON is ReencryptEncValue for a JSON object column (LLM
// credential metadata, a submission's payload): it re-encrypts the top-level
// string values that are wholly a legacy or v2 EncryptValue output, which is
// where those writers put them, and re-marshals the object. Numbers keep
// their text. It returns the new document and how many values changed;
// anything that is not a JSON object comes back unchanged.
func ReencryptEncJSON(doc string) (string, int) {
	if !strings.Contains(doc, "$ENC/") {
		return doc, 0
	}
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return doc, 0
	}
	n := 0
	for k, v := range obj {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if enc, changed := ReencryptEncValue(s); changed {
			obj[k] = enc
			n++
		}
	}
	if n == 0 {
		return doc, 0
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(obj); err != nil {
		return doc, 0
	}
	return strings.TrimSuffix(buf.String(), "\n"), n
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
