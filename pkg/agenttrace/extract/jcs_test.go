package extract

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RFC 8785 §3.2.2: the sample input and its canonical form.
func TestCanonicalize_RFC8785Sample(t *testing.T) {
	in := `{
  "numbers": [333333333.33333329, 1E30, 4.50, 2e-3, 0.000000000000000000000000001],
  "string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/",
  "literals": [null, true, false]
}`
	want := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B\"\\\\\"/"}`
	got, err := Canonicalize([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

// RFC 8785 §3.2.3: members sort by UTF-16 code units, so the emoji (a
// surrogate pair, 0xd83d...) sorts before U+FB33 although its code point is
// higher.
func TestCanonicalize_RFC8785SortOrder(t *testing.T) {
	in := `{"\u20ac":"Euro Sign","\r":"Carriage Return","\ufb33":"Hebrew Letter Dalet With Dagesh","1":"One","\ud83d\ude00":"Emoji: Grinning Face","\u0080":"Control","\u00f6":"Latin Small Letter O With Diaeresis"}`
	want := "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"\u00f6\":\"Latin Small Letter O With Diaeresis\",\"\u20ac\":\"Euro Sign\",\"\U0001F600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}"
	got, err := Canonicalize([]byte(in))
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

// RFC 8785 Appendix B: IEEE 754 doubles and their ECMAScript serialisation.
func TestFormatNumber_RFC8785AppendixB(t *testing.T) {
	vectors := []struct{ ieee, want string }{
		{"0000000000000000", "0"},
		{"8000000000000000", "0"},
		{"0000000000000001", "5e-324"},
		{"8000000000000001", "-5e-324"},
		{"7fefffffffffffff", "1.7976931348623157e+308"},
		{"ffefffffffffffff", "-1.7976931348623157e+308"},
		{"4340000000000000", "9007199254740992"},
		{"c340000000000000", "-9007199254740992"},
		{"4430000000000000", "295147905179352830000"},
		{"44b52d02c7e14af5", "9.999999999999997e+22"},
		{"44b52d02c7e14af6", "1e+23"},
		{"44b52d02c7e14af7", "1.0000000000000001e+23"},
		{"444b1ae4d6e2ef4e", "999999999999999700000"},
		{"444b1ae4d6e2ef4f", "999999999999999900000"},
		{"444b1ae4d6e2ef50", "1e+21"},
		{"3eb0c6f7a0b5ed8c", "9.999999999999997e-7"},
		{"3eb0c6f7a0b5ed8d", "0.000001"},
		{"41b3de4355555553", "333333333.3333332"},
		{"41b3de4355555554", "333333333.33333325"},
		{"41b3de4355555555", "333333333.3333333"},
		{"41b3de4355555556", "333333333.3333334"},
		{"41b3de4355555557", "333333333.33333343"},
		{"becbf647612f3696", "-0.0000033333333333333333"},
		{"43143ff3c1cb0959", "1424953923781206.2"},
	}
	for _, v := range vectors {
		raw, err := hex.DecodeString(v.ieee)
		require.NoError(t, err)
		f := math.Float64frombits(binary.BigEndian.Uint64(raw))
		got, err := formatNumber(strconv.FormatFloat(f, 'g', -1, 64))
		require.NoError(t, err, v.ieee)
		assert.Equal(t, v.want, got, v.ieee)
	}
}

func TestCanonicalize_Errors(t *testing.T) {
	_, err := Canonicalize([]byte(`{"a":1,"a":2}`))
	assert.ErrorIs(t, err, ErrDuplicateKey)

	_, err = Canonicalize([]byte(`[1e400]`))
	assert.ErrorIs(t, err, ErrNumber)

	deep := make([]byte, 0, 2*(maxDepth+2))
	for i := 0; i < maxDepth+2; i++ {
		deep = append(deep, '[')
	}
	for i := 0; i < maxDepth+2; i++ {
		deep = append(deep, ']')
	}
	_, err = Canonicalize(deep)
	assert.ErrorIs(t, err, ErrTooDeep)

	_, err = Canonicalize([]byte(`{"a":1} {"b":2}`))
	assert.Error(t, err)
	_, err = Canonicalize([]byte(`{"a":`))
	assert.Error(t, err)
}

func TestCanonicalize_ControlCharsAndNoHTMLEscape(t *testing.T) {
	got, err := Canonicalize([]byte(`{"k":"<a>&\u0001\u001f\u007f"}`))
	require.NoError(t, err)
	assert.Equal(t, "{\"k\":\"<a>&\\u0001\\u001f\u007f\"}", string(got))
}
