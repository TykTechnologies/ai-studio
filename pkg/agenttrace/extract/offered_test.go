package extract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tools a request offers the model are extracted per format, in body
// order, alongside its tool results (jyx.51).
func TestParseRequest_OfferedTools(t *testing.T) {
	for _, tc := range []struct {
		format, body string
		want         []string
	}{
		{FormatOpenAI, `{"messages":[{"role":"user","content":"hi"}],"tools":[` +
			`{"type":"function","function":{"name":"search","parameters":{"type":"object"}}},` +
			`{"type":"function","function":{"name":"mcp__crm__find_contact","parameters":{"type":"object"}}}]}`,
			[]string{"search", "mcp__crm__find_contact"}},
		{FormatAnthropic, `{"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"fetch","input_schema":{"type":"object"}}]}`,
			[]string{"fetch"}},
		{FormatGoogleAI, `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"lookup"},{"name":"book"}]}]}`,
			[]string{"lookup", "book"}},
		{FormatBedrock, `{"messages":[{"role":"user","content":[{"text":"hi"}]}],"toolConfig":{"tools":[{"toolSpec":{"name":"weather","inputSchema":{"json":{}}}}]}}`,
			[]string{"weather"}},
		{FormatOpenAIResponses, `{"input":"hi","tools":[{"type":"function","name":"search"},{"type":"web_search"},{"type":"mcp","server_label":"crm"}]}`,
			[]string{"search", "web_search", "mcp:crm"}},
		{FormatOpenAI, `{"messages":[{"role":"user","content":"hi"}]}`, nil},
	} {
		t.Run(tc.format, func(t *testing.T) {
			r, err := ParseRequestExtracts(tc.format, []byte(tc.body), testFP)
			require.NoError(t, err)
			assert.Equal(t, tc.want, r.OfferedTools)
		})
	}
}

// The offered set is bounded and deduplicated.
func TestBoundNames(t *testing.T) {
	names := []string{"a", "a", "", "b"}
	for i := 0; i < 2*MaxOfferedTools; i++ {
		names = append(names, "t"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	got := boundNames(names)
	assert.Equal(t, "a", got[0])
	assert.Equal(t, "b", got[1])
	assert.Len(t, got, MaxOfferedTools)
}
