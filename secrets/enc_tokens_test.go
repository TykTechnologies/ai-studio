package secrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReencryptEncValues_ToV3InFIPSMode(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	forceStandard(t)
	v2 := EncryptValue("api-key-value")
	require.True(t, strings.HasPrefix(v2, "$ENC/v2:"))
	legacy := "$ENC/" + legacyFixture // "fips-fixture-plaintext"

	forceFIPS(t)
	// A whole-value column.
	got, n := ReencryptEncValues(v2)
	assert.Equal(t, 1, n)
	assert.True(t, strings.HasPrefix(got, "$ENC/v3:"), got)
	assert.Equal(t, "api-key-value", DecryptValue(got))

	got, n = ReencryptEncValues(legacy)
	assert.Equal(t, 1, n)
	assert.Equal(t, "fips-fixture-plaintext", DecryptValue(got))

	// A JSON document with several values, a v3 one and plain ones.
	v3 := EncryptValue("already-v3")
	doc, _ := json.Marshal(map[string]string{"a": v2, "b": legacy, "c": v3, "d": "plain", "e": "$SECRET/REF"})
	out, n := ReencryptEncValues(string(doc))
	assert.Equal(t, 2, n)
	var m map[string]string
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	assert.Equal(t, "api-key-value", DecryptValue(m["a"]))
	assert.True(t, strings.HasPrefix(m["a"], "$ENC/v3:"))
	assert.Equal(t, "fips-fixture-plaintext", DecryptValue(m["b"]))
	assert.Equal(t, v3, m["c"], "v3 values are left alone")
	assert.Equal(t, "plain", m["d"])
	assert.Equal(t, "$SECRET/REF", m["e"])

	// Nothing to do.
	same, n := ReencryptEncValues(out)
	assert.Zero(t, n)
	assert.Equal(t, out, same)
}

func TestReencryptEncValues_LeavesForeignValues(t *testing.T) {
	t.Setenv(midsommarSecret, "fixture-key")
	forceStandard(t)
	t.Setenv(midsommarSecret, "another-key")
	foreign := EncryptValue("other key's value") // v2: authenticated, fails under fixture-key
	t.Setenv(midsommarSecret, "fixture-key")
	forceFIPS(t)
	got, n := ReencryptEncValues(foreign)
	assert.Zero(t, n)
	assert.Equal(t, foreign, got)

	t.Setenv(midsommarSecret, "")
	got, n = ReencryptEncValues("$ENC/" + legacyFixture)
	assert.Zero(t, n, "no key: nothing changes")
	assert.Equal(t, "$ENC/"+legacyFixture, got)
}
