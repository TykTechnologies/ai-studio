package secrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReencryptEncValue_WholeValuesToV3InFIPSMode(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	forceStandard(t)
	v2 := EncryptValue("api-key-value")
	require.True(t, strings.HasPrefix(v2, "$ENC/v2:"))
	legacy := "$ENC/" + legacyFixture // "fips-fixture-plaintext"

	forceFIPS(t)
	got, ok := ReencryptEncValue(v2)
	assert.True(t, ok)
	assert.True(t, strings.HasPrefix(got, "$ENC/v3:"), got)
	assert.Equal(t, "api-key-value", DecryptValue(got))

	got, ok = ReencryptEncValue(legacy)
	assert.True(t, ok)
	assert.Equal(t, "fips-fixture-plaintext", DecryptValue(got))

	v3 := EncryptValue("already-v3")
	for _, v := range []string{
		v3,
		"plain",
		"$SECRET/REF",
		// Text that merely contains a token is not a value we wrote: the
		// unauthenticated legacy reader would turn it into garbage.
		"note: $ENC/AAAAAAAAAAAAAAAAAAAAAAAAAAAA end",
		"prefix " + v2,
		v2 + " suffix",
	} {
		same, ok := ReencryptEncValue(v)
		assert.False(t, ok, v)
		assert.Equal(t, v, same)
	}
}

func TestReencryptEncJSON(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	forceStandard(t)
	v2 := EncryptValue("api-key-value")
	legacy := "$ENC/" + legacyFixture

	forceFIPS(t)
	v3 := EncryptValue("already-v3")
	note := "note: $ENC/AAAAAAAAAAAAAAAAAAAAAAAAAAAA end"
	doc := `{"a":"` + v2 + `","b":"` + legacy + `","c":"` + v3 + `","d":"plain","e":"$SECRET/REF","note":"` + note +
		`","big":12345678901234567890,"nested":{"x":"` + v2 + `"},"html":"<b>&</b>"}`

	out, n := ReencryptEncJSON(doc)
	assert.Equal(t, 2, n, "the two top-level whole values")
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(out))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&m))
	assert.True(t, strings.HasPrefix(m["a"].(string), "$ENC/v3:"))
	assert.Equal(t, "api-key-value", DecryptValue(m["a"].(string)))
	assert.Equal(t, "fips-fixture-plaintext", DecryptValue(m["b"].(string)))
	assert.Equal(t, v3, m["c"])
	assert.Equal(t, "plain", m["d"])
	assert.Equal(t, "$SECRET/REF", m["e"])
	assert.Equal(t, note, m["note"], "free text containing a token is left alone")
	assert.Equal(t, json.Number("12345678901234567890"), m["big"], "numbers keep their text")
	assert.Equal(t, v2, m["nested"].(map[string]any)["x"], "only top-level values, as the writers store them")
	assert.Equal(t, "<b>&</b>", m["html"])

	same, n := ReencryptEncJSON(out)
	assert.Zero(t, n)
	assert.Equal(t, out, same)

	for _, notObj := range []string{`[` + `"` + v2 + `"]`, `"` + v2 + `"`, `not json ` + v2} {
		same, n := ReencryptEncJSON(notObj)
		assert.Zero(t, n, notObj)
		assert.Equal(t, notObj, same)
	}
}

func TestReencryptEncValue_LeavesForeignValues(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	forceStandard(t)
	t.Setenv(midsommarSecret, "another-key")
	foreign := EncryptValue("other key's value") // v2: authenticated, fails under fixture-key
	t.Setenv(midsommarSecret, "fixture-key")
	forceFIPS(t)
	got, ok := ReencryptEncValue(foreign)
	assert.False(t, ok)
	assert.Equal(t, foreign, got)

	t.Setenv(midsommarSecret, "")
	got, ok = ReencryptEncValue("$ENC/" + legacyFixture)
	assert.False(t, ok, "no key: nothing changes")
	assert.Equal(t, "$ENC/"+legacyFixture, got)
}
