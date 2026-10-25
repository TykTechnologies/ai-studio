package extract

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/testinfra/toolfixtures"
)

// trimView is what the capture keeps of a request (Request.Apply) and the
// turn boundary.
type trimView struct {
	X        agenttrace.Extracts
	LastRole string
}

func viewOf(r Request) trimView {
	var v trimView
	r.Apply(&v.X)
	v.LastRole = r.LastRole
	return v
}

// referenceRequest is parseRequest without the history trim: the whole
// body segmented.
func referenceRequest(format string, body []byte) (Request, error) {
	format = strings.ToLower(format)
	var r Request
	var err error
	if format == FormatOpenAIResponses {
		r, err = parseResponsesRequest(body, testFP)
	} else {
		r, _, err = parseUnits(format, body, testFP)
	}
	r.bound()
	return r, err
}

// assertSameAsReference checks ParseRequest against the reference: equal
// whenever the reference parses the body. It reports whether the trimmed
// path was taken.
func assertSameAsReference(t *testing.T, name, format string, body []byte) (trimmed bool) {
	t.Helper()
	want, werr := referenceRequest(format, body)
	got, gerr := ParseRequest(format, body, testFP)
	if werr != nil {
		return false
	}
	require.NoError(t, gerr, name)
	assert.Equal(t, viewOf(want), viewOf(got), "%s\n%s", name, body)
	if tb, ok := trimHistory(strings.ToLower(format), body); ok {
		_, settled, err := parseUnits(strings.ToLower(format), tb, testFP)
		return err == nil && settled
	}
	return false
}

func TestTrimHistory_Fixtures(t *testing.T) {
	for _, f := range toolfixtures.ByFormat(requestFormats...) {
		assertSameAsReference(t, f.Name, f.Format, f.Body)
	}
	assertSameAsReference(t, "big anthropic", FormatAnthropic, bigAnthropicRequest())
}

// convo builds random conversations in each dialect: user text, system
// messages, assistant text, assistant tool calls, tool results answering
// recent or old calls (some failed), and empty messages.
type convo struct {
	rnd   *rand.Rand
	calls []string
	n     int
}

func (c *convo) id() string { c.n++; return fmt.Sprintf("c%d", c.n) }

func (c *convo) pick() string {
	if len(c.calls) == 0 {
		return "none"
	}
	if c.rnd.Intn(4) == 0 {
		return c.calls[c.rnd.Intn(len(c.calls))] // an old call
	}
	return c.calls[len(c.calls)-1]
}

func (c *convo) body(format string) []byte {
	steps := 1 + c.rnd.Intn(12)
	var msgs []string
	for s := 0; s < steps; s++ {
		k := c.rnd.Intn(7)
		switch format {
		case FormatOpenAI, FormatOllama:
			switch k {
			case 0:
				msgs = append(msgs, `{"role":"system","content":"sys"}`)
			case 1, 2:
				msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":"u%d"}`, s))
			case 3:
				msgs = append(msgs, `{"role":"assistant","content":"a"}`)
			case 4:
				id := c.id()
				c.calls = append(c.calls, id)
				if format == FormatOllama {
					msgs = append(msgs, `{"role":"assistant","content":"","tool_calls":[{"function":{"name":"t","arguments":{}}}]}`)
				} else {
					msgs = append(msgs, fmt.Sprintf(`{"role":"assistant","content":null,"tool_calls":[{"id":%q,"type":"function","function":{"name":"t","arguments":"{}"}}]}`, id))
				}
			case 5:
				if format == FormatOllama {
					msgs = append(msgs, fmt.Sprintf(`{"role":"tool","content":"r%d"}`, s))
				} else {
					msgs = append(msgs, fmt.Sprintf(`{"role":"tool","tool_call_id":%q,"content":"r%d"}`, c.pick(), s))
				}
			case 6:
				msgs = append(msgs, `{"role":"user","content":""}`)
			}
		case FormatAnthropic, FormatBedrock:
			bedrock := format == FormatBedrock
			switch k {
			case 0, 1:
				if bedrock {
					msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":[{"text":"u%d"}]}`, s))
				} else {
					msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":"u%d"}`, s))
				}
			case 2, 3:
				if bedrock {
					msgs = append(msgs, `{"role":"assistant","content":[{"text":"a"}]}`)
				} else {
					msgs = append(msgs, `{"role":"assistant","content":[{"type":"text","text":"a"}]}`)
				}
			case 4:
				id := c.id()
				c.calls = append(c.calls, id)
				if bedrock {
					msgs = append(msgs, fmt.Sprintf(`{"role":"assistant","content":[{"toolUse":{"toolUseId":%q,"name":"t","input":{}}}]}`, id))
				} else {
					msgs = append(msgs, fmt.Sprintf(`{"role":"assistant","content":[{"type":"tool_use","id":%q,"name":"t","input":{}}]}`, id))
				}
			case 5, 6:
				failed := c.rnd.Intn(3) == 0
				if bedrock {
					status := "success"
					if failed {
						status = "error"
					}
					msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":[{"toolResult":{"toolUseId":%q,"content":[{"text":"r%d"}],"status":%q}}]}`, c.pick(), s, status))
				} else {
					msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":"r%d","is_error":%t}]}`, c.pick(), s, failed))
				}
			}
		default: // Gemini
			switch k {
			case 0, 1, 2:
				msgs = append(msgs, fmt.Sprintf(`{"role":"user","parts":[{"text":"u%d"}]}`, s))
			case 3:
				msgs = append(msgs, `{"role":"model","parts":[{"text":"a"}]}`)
			case 4:
				msgs = append(msgs, `{"role":"model","parts":[{"functionCall":{"name":"t","args":{}}}]}`)
			case 5, 6:
				msgs = append(msgs, fmt.Sprintf(`{"role":"user","parts":[{"functionResponse":{"name":"t","response":{"r":%d}}}]}`, s))
			}
		}
	}
	list := strings.Join(msgs, ",")
	switch format {
	case FormatOpenAI, FormatOllama:
		return []byte(`{"model":"m","tools":[{"type":"function","function":{"name":"t","parameters":{"type":"object"}}}],"messages":[` + list + `]}`)
	case FormatAnthropic:
		sys := ""
		if c.rnd.Intn(2) == 0 {
			sys = `"system":"be brief",`
		}
		return []byte(`{"model":"m",` + sys + `"tools":[{"name":"t","input_schema":{"type":"object"}}],"messages":[` + list + `]}`)
	case FormatBedrock:
		return []byte(`{"toolConfig":{"tools":[{"toolSpec":{"name":"t","inputSchema":{"json":{"type":"object"}}}}]},"messages":[` + list + `]}`)
	}
	return []byte(`{"tools":[{"functionDeclarations":[{"name":"t"}]}],"contents":[` + list + `]}`)
}

// jyx.57: the trimmed parse equals the whole-body parse over thousands of
// random conversations in every dialect, and the trim is actually taken.
func TestTrimHistory_Differential(t *testing.T) {
	// The formats share nothing (each has its own generator), so they run
	// side by side: this is the slowest test in the package under -race.
	for _, format := range []string{FormatOpenAI, FormatOllama, FormatAnthropic, FormatBedrock, FormatGoogleAI, FormatVertex} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			rnd := rand.New(rand.NewSource(57))
			taken, parsed := 0, 0
			for i := 0; i < 3000; i++ {
				c := &convo{rnd: rnd}
				body := c.body(format)
				if _, err := referenceRequest(format, body); err == nil {
					parsed++
				}
				if assertSameAsReference(t, fmt.Sprintf("%s #%d", format, i), format, body) {
					taken++
				}
			}
			assert.Greater(t, parsed, 2500, "%s: the generator must make bodies the segmenter accepts", format)
			assert.Greater(t, taken, 500, "%s: the trim must be exercised", format)
		})
	}
}

// A role or array key that is escaped, repeated or not a string is never a
// cut; nor is a body that is not valid JSON.
func TestTrimHistory_Conservative(t *testing.T) {
	tail := `{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"r"}]}`
	call := `{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"t","input":{}}]}`
	for name, body := range map[string]string{
		"escaped role":    `{"messages":[{"role":"user","content":"u"},{"role":"assist\u0061nt","content":[]},` + tail + `]}`,
		"repeated role":   `{"messages":[{"role":"user","content":"u"},{"role":"assistant","role":"assistant","content":[]},` + tail + `]}`,
		"role not string": `{"messages":[{"role":"user","content":"u"},{"role":["assistant"],"content":[]},` + tail + `]}`,
		"escaped key":     `{"m\u0065ssages":[{"role":"user","content":"u"},` + call + `,` + tail + `]}`,
		"repeated key":    `{"messages":[],"messages":[{"role":"user","content":"u"},` + call + `,` + tail + `]}`,
		"invalid json":    `{"messages":[{"role":"user","content":"u"},` + call + `,` + tail + `]`,
		"not an array":    `{"messages":{"role":"assistant"}}`,
		"first message":   `{"messages":[` + call + `,` + tail + `]}`,
	} {
		_, ok := trimHistory(FormatAnthropic, []byte(body))
		assert.False(t, ok, name)
		assertSameAsReference(t, name, FormatAnthropic, []byte(body))
	}

	// Spacing, nesting and strings holding brackets or quotes do not move
	// the cut.
	body := " {\n \"model\" : \"m\\\"]}\" , \"messages\" : [ {\"role\":\"user\",\"content\":\"[{\\\"role\\\":\\\"assistant\\\"}]\"} ,\n" +
		call + " , " + tail + " ] , \"x\" : [1, {\"y\": null}] } "
	got, ok := trimHistory(FormatAnthropic, []byte(body))
	require.True(t, ok)
	assert.JSONEq(t, `{"model":"m\"]}","messages":[`+call+`,`+tail+`],"x":[1,{"y":null}]}`, string(got))
	assert.True(t, assertSameAsReference(t, "spacing", FormatAnthropic, []byte(body)))
}

// A request ending in the model's message (a prefill) or holding no new
// input after the last call falls back to the whole body.
func TestTrimHistory_FallsBack(t *testing.T) {
	for name, body := range map[string]string{
		"prefill": `{"system":"s","messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c0","content":"r"}]},` +
			`{"role":"assistant","content":"a"},{"role":"user","content":"u"},{"role":"assistant","content":"Sure"}]}`,
		"empty tail": `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c0","name":"t","input":{}}]},` +
			`{"role":"user","content":[{"type":"tool_result","tool_use_id":"c0","content":"r"}]},{"role":"assistant","content":"a"},{"role":"user","content":[]}]}`,
	} {
		assertSameAsReference(t, name, FormatAnthropic, []byte(body))
	}
}

// Every prefix of a valid body is malformed in some way: the scanner gives
// up or the segmenter refuses, and nothing panics.
func TestTrimHistory_Truncated(t *testing.T) {
	body := (&convo{rnd: rand.New(rand.NewSource(3))}).body(FormatAnthropic)
	body = append(body[:len(body)-2:len(body)-2], []byte(`,{"role":"assistant","content":"a\\\"b"},{"role":"user","content":"u"}]}`)...)
	for n := 0; n < len(body); n++ {
		_, _ = trimHistory(FormatAnthropic, body[:n])
		assertSameAsReference(t, fmt.Sprintf("prefix %d", n), FormatAnthropic, body[:n])
	}
}

// Hostile shapes stay linear: deep nesting, long backslash runs, a sea of
// quotes and many tiny messages, 1 MiB each.
func TestTrimHistory_Linear(t *testing.T) {
	const mib = 1 << 20
	tail := `,{"role":"assistant","content":"a"},{"role":"user","content":"u"}]}`
	for name, body := range map[string]string{
		"deep nesting":   `{"messages":[{"role":"user","content":` + strings.Repeat("[", mib/2) + strings.Repeat("]", mib/2) + `}` + tail,
		"backslash runs": `{"messages":[{"role":"user","content":"` + strings.Repeat(`\\\\\\\\\"`, mib/10) + `"}` + tail,
		"quotes":         `{"messages":[{"role":"user","content":[` + strings.Repeat(`"",`, mib/3) + `""]}` + tail,
		"tiny messages":  `{"messages":[` + strings.Repeat(`{"role":"assistant"},`, mib/20) + `{}` + tail,
		"unclosed":       `{"messages":[{"role":"user","content":"` + strings.Repeat(`\"`, mib/2),
	} {
		start := time.Now()
		_, _ = trimHistory(FormatAnthropic, []byte(body))
		assert.Less(t, time.Since(start), 200*time.Millisecond, name)
	}
}

// A history the segmenter would refuse is not read: an old message with a
// repeated key fails the whole-body parse but not the trimmed one, which
// returns the trailing results as if it were well formed.
func TestTrimHistory_OldHistoryNotRead(t *testing.T) {
	tail := `{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"t","input":{}}]},` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"r"}]}]}`
	broken := []byte(`{"messages":[{"role":"user","content":"u","content":"v"},` + tail)
	_, err := referenceRequest(FormatAnthropic, broken)
	require.Error(t, err)
	got, err := ParseRequest(FormatAnthropic, broken, testFP)
	require.NoError(t, err)
	want, err := referenceRequest(FormatAnthropic, []byte(`{"messages":[{"role":"user","content":"u"},`+tail))
	require.NoError(t, err)
	assert.Equal(t, viewOf(want), viewOf(got))
}

// Untrusted bodies: the trimmed parse never panics and equals the
// reference whenever the reference parses.
func FuzzTrimHistory(f *testing.F) {
	for _, fx := range toolfixtures.ByFormat(requestFormats...) {
		f.Add(fx.Format, fx.Body)
	}
	rnd := rand.New(rand.NewSource(1))
	for _, format := range []string{FormatOpenAI, FormatAnthropic, FormatBedrock, FormatGoogleAI} {
		f.Add(format, (&convo{rnd: rnd}).body(format))
	}
	f.Fuzz(func(t *testing.T, format string, body []byte) {
		assertSameAsReference(t, "fuzz", format, body)
	})
}

// longAgentRequest is an Anthropic agent loop of turns tool round trips,
// each with ~1 KiB of tool output.
func longAgentRequest(turns int) []byte {
	var b strings.Builder
	b.WriteString(`{"model":"m","system":"You are an agent.","tools":[{"name":"t","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"start"}`)
	out := strings.Repeat("output line with some words ", 36)
	for i := 0; i < turns; i++ {
		fmt.Fprintf(&b, `,{"role":"assistant","content":[{"type":"text","text":"step %d"},{"type":"tool_use","id":"toolu_%d","name":"t","input":{"q":"x"}}]}`, i, i)
		fmt.Fprintf(&b, `,{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_%d","content":%q}]}`, i, out)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

func BenchmarkParseRequest_AnthropicAgent256KiB(b *testing.B) {
	body := longAgentRequest(220)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseRequest(FormatAnthropic, body, testFP); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseRequest_AnthropicAgent256KiB_Reference(b *testing.B) {
	body := longAgentRequest(220)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := referenceRequest(FormatAnthropic, body); err != nil {
			b.Fatal(err)
		}
	}
}
