package extract

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFP = agenttrace.StaticFingerprinter{ID: 7, Key: []byte("extract-test-key")}

func argsFP(t *testing.T, args string) agenttrace.Fingerprint {
	t.Helper()
	f, err := ArgsFingerprint(testFP, []byte(args))
	require.NoError(t, err)
	require.Len(t, f, 33)
	assert.Equal(t, byte(7), f.KeyID())
	return f
}

// Studio's OpenAPI-to-MCP shim flattens object parameters (body.name is
// published as body_name), so an LLM call against the REST shape and the MCP
// tools/call of the same operation must fingerprint equal (R2).
func TestArgsFingerprint_FlatMCPEqualsNestedREST(t *testing.T) {
	nested := `{"id":"42","body":{"name":"Sam","address":{"city":"Oslo"},"tags":["a","b"],"empty":{}}}`
	flat := `{"body_address_city":"Oslo","body_tags":["a","b"],"id":"42","body_name":"Sam"}`
	assert.Equal(t, argsFP(t, nested), argsFP(t, flat))
}

// OpenAI sends function.arguments as a JSON string; Anthropic sends input as
// an object. Key order and whitespace are not differences.
// jyx.63: members that flatten to one key would make JCS depend on order,
// so such arguments get no fingerprint.
func TestArgsFingerprint_FlattenCollision(t *testing.T) {
	for _, args := range []string{`{"a":{"b":1},"a_b":2}`, `{"a_b":2,"a":{"b":1}}`, `{"x":{"y_z":1,"y":{"z":2}}}`} {
		_, err := ArgsFingerprint(testFP, []byte(args))
		assert.ErrorIs(t, err, ErrDuplicateKey, args)
	}
}

func TestCanonicalize_ManyKeysFast(t *testing.T) {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 200000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"k%07d":%d`, 199999-i, i)
	}
	b.WriteString("}")
	start := time.Now()
	out, err := Canonicalize([]byte(b.String()))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), `{"k0000000":199999,`))
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestArgsFingerprint_StringEncodedEqualsObject(t *testing.T) {
	obj := `{"query":"cycle lanes","limit":5}`
	str := `"{ \"limit\": 5.0, \"query\": \"cycle lanes\" }"`
	assert.Equal(t, argsFP(t, obj), argsFP(t, str))
}

func TestArgsFingerprint_ValuesMatter(t *testing.T) {
	assert.NotEqual(t, argsFP(t, `{"q":"a"}`), argsFP(t, `{"q":"b"}`))
	assert.NotEqual(t, argsFP(t, `{"q":"1"}`), argsFP(t, `{"q":1}`))
}

func TestArgsFingerprint_EmptyAndInvalid(t *testing.T) {
	assert.Equal(t, argsFP(t, ``), argsFP(t, `{}`))
	_, err := ArgsFingerprint(testFP, []byte(`{"q":`))
	assert.Error(t, err)
	_, err = ArgsFingerprint(testFP, []byte(`"{\"a\":1,\"a\":2}"`))
	assert.ErrorIs(t, err, ErrDuplicateKey)

	// A custom tool's free-form input is a plain string.
	assert.NotEqual(t, argsFP(t, `"teh quick"`), argsFP(t, `"the quick"`))
}

func TestFingerprints_NilFingerprinter(t *testing.T) {
	f, err := ArgsFingerprint(nil, []byte(`{"a":1}`))
	assert.NoError(t, err)
	assert.Nil(t, f)
	assert.Nil(t, ResultFingerprint(nil, "x"))
	assert.Nil(t, TextFingerprint(nil, "x"))
}

// The kinds are domain-separated: the same bytes as arguments, a result and
// a text give three different fingerprints.
func TestFingerprints_DomainSeparated(t *testing.T) {
	a := argsFP(t, `"hello"`)
	r := ResultFingerprint(testFP, `"hello"`)
	x := TextFingerprint(testFP, `"hello"`)
	assert.NotEqual(t, a, r)
	assert.NotEqual(t, r, x)
	assert.NotEqual(t, a, x)
}

func TestResultFingerprint_BlockSplitAndWhitespace(t *testing.T) {
	one := ResultFingerprint(testFP, "Build #812 failed\n\nat step lint.")
	two := ResultFingerprint(testFP, "Build #812 failed", "at step  lint.")
	assert.Equal(t, one, two)
	assert.Nil(t, ResultFingerprint(testFP, "  ", "\n"))
	assert.NotEqual(t, one, ResultFingerprint(testFP, "Build #813 failed at step lint."))
}

func TestNormalizeText(t *testing.T) {
	assert.Equal(t, "a b c", NormalizeText("  a\t\nb   c  "))
	// NFC: e + combining acute equals the precomposed é.
	assert.Equal(t, NormalizeText("café"), NormalizeText("café"))
	assert.Equal(t, "", NormalizeText(" \n\t"))
}

func TestNameNormalizer(t *testing.T) {
	n := NewNameNormalizer(DefaultToolNamePrefixes, []string{"CRM", "github"})
	cases := map[string]string{
		"mcp__crm__find_contact": "find_contact",
		"mcp__any_server__do_x":  "do_x",
		"crm__find_contact":      "find_contact",
		"GitHub.create_issue":    "create_issue",
		"crm_find_contact":       "find_contact",
		"get_weather":            "get_weather",
		"crm__":                  "crm__",
		"Find_Contact":           "find_contact",
		"mcp__crm__":             "mcp__crm__",
	}
	for in, want := range cases {
		assert.Equal(t, want, n.Normalize(in), in)
	}

	// Without known servers, {server}-led templates are skipped: they
	// would strip the first word of any name.
	bare := NewNameNormalizer(DefaultToolNamePrefixes, nil)
	assert.Equal(t, "get_weather", bare.Normalize("get_weather"))
	assert.Equal(t, "find_contact", bare.Normalize("mcp__crm__find_contact"))

	var nilN *NameNormalizer
	assert.Equal(t, "x", nilN.Normalize(" X "))
}

func TestBinaryString(t *testing.T) {
	mt, ok := BinaryString("data:image/PNG;base64,iVBORw0KGgo=")
	assert.True(t, ok)
	assert.Equal(t, "image/png", mt)

	_, ok = BinaryString(strings.Repeat("QUJD", 300))
	assert.True(t, ok)
	_, ok = BinaryString(strings.Repeat("QUJD", 200))
	assert.False(t, ok, "base64 under 1 KiB stays inline")
	_, ok = BinaryString(strings.Repeat("hello world ", 200))
	assert.False(t, ok)

	// jyx.146: an SSE body starts with "data:" and holds a comma, but is
	// text; so is anything whose "media type" is not a type/subtype token.
	for _, s := range []string{
		"data: {\"id\":\"c2\",\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n\n",
		`data:{"id":"c2","object":"chat.completion.chunk"}`,
		"data:text plain,x",
		"data:not-a-type,x",
	} {
		_, ok = BinaryString(s)
		assert.False(t, ok, "%q", s)
	}
	mt, ok = BinaryString("data:,hello")
	assert.True(t, ok)
	assert.Equal(t, "text/plain", mt)

	assert.True(t, BinaryMediaType("image/jpeg"))
	assert.True(t, BinaryMediaType("application/pdf"))
	assert.False(t, BinaryMediaType("text/plain"))
	assert.False(t, BinaryMediaType("application/json"))
	assert.False(t, BinaryMediaType(""))
}
