package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// The fast path keeps exactly what the capture applies: for every fixture,
// ParseRequestExtracts and ParseRequest give the same extracts (jyx.149).
func TestParseRequestExtracts_MatchesParseRequest(t *testing.T) {
	fixtures := toolfixtures.ByFormat(requestFormats...)
	require.NotEmpty(t, fixtures)
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			full, errFull := ParseRequest(f.Format, f.Body, testFP)
			fast, errFast := ParseRequestExtracts(f.Format, f.Body, testFP)
			assert.Equal(t, errFull == nil, errFast == nil)
			var want, got agenttrace.Extracts
			full.Apply(&want)
			fast.Apply(&got)
			assert.Equal(t, want, got)
		})
	}
}

// A chat request with no tool result skips the parse; one with a result
// still gets it.
func TestParseRequestExtracts_SkipsBodiesWithoutResults(t *testing.T) {
	chat := []byte(`{"messages":[{"role":"user","content":"hi"}]}`)
	r, err := ParseRequestExtracts(FormatOpenAI, chat, testFP)
	require.NoError(t, err)
	assert.Empty(t, r.ToolResults)

	withResult := []byte(`{"messages":[{"role":"user","content":"find x"},` +
		`{"role":"assistant","tool_calls":[{"id":"call_0","type":"function","function":{"name":"search","arguments":"{}"}}]},` +
		`{"role":"tool","tool_call_id":"call_0","content":"found"}]}`)
	r, err = ParseRequestExtracts(FormatOpenAI, withResult, testFP)
	require.NoError(t, err)
	require.Len(t, r.ToolResults, 1)
	assert.Equal(t, "call_0", r.ToolResults[0].ID)
}
