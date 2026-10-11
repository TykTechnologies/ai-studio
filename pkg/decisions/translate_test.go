package decisions

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const openAIRequest = `{
  "model": "jev-latest",
  "input": "Customer message: I was charged twice for my order last week.",
  "questions": [
    {"type": "predicate", "name": "angry", "instructions": "Is the customer angry?"},
    {"type": "choice", "name": "topic", "instructions": "Which team handles this?",
     "choices": [{"value": "shipping", "description": "deliveries"}, {"value": "billing", "description": "payments, charges, refunds"}, {"value": "other"}]},
    {"type": "score", "name": "urgency", "instructions": "How urgent?",
     "levels": [{"label": "low"}, {"label": "medium", "description": "this week"}, {"label": "high", "description": "today"}]}
  ]
}`

func mustRequest(t *testing.T, body string) *Request {
	t.Helper()
	req, err := ParseRequest([]byte(body), FormatOpenAI)
	require.NoError(t, err)
	return req
}

func TestToSystemOne_PreservesOrderAndMapsTypes(t *testing.T) {
	so, err := ToSystemOne(mustRequest(t, openAIRequest))
	require.NoError(t, err)

	body, err := json.Marshal(so)
	require.NoError(t, err)
	got := string(body)

	// Question order and choice order survive marshalling (a Go map would sort them).
	assert.Less(t, strings.Index(got, `"angry"`), strings.Index(got, `"topic"`))
	assert.Less(t, strings.Index(got, `"topic"`), strings.Index(got, `"urgency"`))
	assert.Less(t, strings.Index(got, `"shipping"`), strings.Index(got, `"billing"`))

	assert.JSONEq(t, `{
	  "model": "jev-latest",
	  "state": "Customer message: I was charged twice for my order last week.",
	  "questions": {
	    "angry": {"type": "noul", "instructions": "Is the customer angry?"},
	    "topic": {"type": "choice", "instructions": "Which team handles this?",
	              "criteria": {"shipping": "deliveries", "billing": "payments, charges, refunds", "other": "other"}},
	    "urgency": {"type": "score", "instructions": "How urgent?",
	                "criteria": ["low", "medium: this week", "high: today"]}
	  }
	}`, got)

	// Round trip back to the OpenAI shape keeps order.
	var back SystemOneRequest
	require.NoError(t, json.Unmarshal(body, &back))
	oa := ToOpenAIRequest(&back)
	require.Len(t, oa.Questions, 3)
	assert.Equal(t, []string{"angry", "topic", "urgency"}, []string{oa.Questions[0].Name, oa.Questions[1].Name, oa.Questions[2].Name})
	assert.Equal(t, TypePredicate, oa.Questions[0].Type)
	assert.Equal(t, "shipping", oa.Questions[1].Choices[0].Value)
	assert.Equal(t, "medium: this week", oa.Questions[2].Levels[1].Label)
}

func TestToSystemOne_MessageInput(t *testing.T) {
	req := mustRequest(t, `{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"first"},{"type":"input_text","text":"second"}]},{"role":"user","content":"third"}],
	  "questions":[{"type":"predicate","name":"q","instructions":"?"}]}`)
	so, err := ToSystemOne(req)
	require.NoError(t, err)
	assert.JSONEq(t, `"first\nsecond\n\nthird"`, string(so.State))
}

func TestToSystemOne_RejectsImages(t *testing.T) {
	req := mustRequest(t, `{"model":"m","input":[{"role":"user","content":[{"type":"input_text","text":"look"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]}],
	  "questions":[{"type":"predicate","name":"q","instructions":"?"}]}`)
	_, err := ToSystemOne(req)
	var verr *ValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, "input", verr.Param)
	assert.Contains(t, verr.Message, "image input")
}

func TestValidate(t *testing.T) {
	choices := func(n int) []Choice {
		out := make([]Choice, n)
		for i := range out {
			out[i] = Choice{Value: strings.Repeat("x", i+1)}
		}
		return out
	}
	levels := func(n int) []Level {
		out := make([]Level, n)
		for i := range out {
			out[i] = Level{Label: "l"}
		}
		return out
	}
	in := json.RawMessage(`"evidence"`)
	cases := []struct {
		name   string
		req    *Request
		target Format
		param  string
	}{
		{"missing model", &Request{Input: in, Questions: []Question{{Type: TypePredicate, Name: "a"}}}, FormatOpenAI, "model"},
		{"missing input", &Request{Model: "m", Questions: []Question{{Type: TypePredicate, Name: "a"}}}, FormatOpenAI, "input"},
		{"no questions", &Request{Model: "m", Input: in}, FormatOpenAI, "questions"},
		{"unnamed", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypePredicate}}}, FormatOpenAI, "questions[0].name"},
		{"duplicate", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypePredicate, Name: "a"}, {Type: TypePredicate, Name: "a"}}}, FormatOpenAI, "questions[1].name"},
		{"bad type", &Request{Model: "m", Input: in, Questions: []Question{{Type: "noul", Name: "a"}}}, FormatOpenAI, "questions[0].type"},
		{"one choice", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypeChoice, Name: "a", Choices: choices(1)}}}, FormatOpenAI, "questions[0].choices"},
		{"repeated choice", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypeChoice, Name: "a", Choices: []Choice{{Value: "x"}, {Value: "x"}}}}}, FormatOpenAI, "questions[0].choices"},
		{"too many choices for systemone", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypeChoice, Name: "a", Choices: choices(256)}}}, FormatSystemOne, "questions[0].choices"},
		{"too many levels for systemone", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypeScore, Name: "a", Levels: levels(11)}}}, FormatSystemOne, "questions[0].levels"},
		{"one level", &Request{Model: "m", Input: in, Questions: []Question{{Type: TypeScore, Name: "a", Levels: levels(1)}}}, FormatOpenAI, "questions[0].levels"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.req, tc.target)
			var verr *ValidationError
			require.True(t, errors.As(err, &verr), "expected a validation error, got %v", err)
			assert.Equal(t, tc.param, verr.Param)
		})
	}

	// OpenAI has no documented 255/10 limits; the gateway leaves those to it.
	assert.NoError(t, Validate(&Request{Model: "m", Input: in, Questions: []Question{{Type: TypeChoice, Name: "a", Choices: choices(256)}}}, FormatOpenAI))
	assert.NoError(t, Validate(mustRequest(t, openAIRequest), FormatSystemOne))
}

const systemOneResponse = `{
  "model": "jev-1.13.0",
  "answers": {
    "urgency": {"type": "score", "score": 1.4, "legend": {"0": "low", "1": "medium: this week", "2": "high: today"},
                "probabilities": [0.2, 0.2, 0.6], "confidence": 0.7},
    "topic": {"type": "choice", "choice": "billing", "probabilities": {"other": 0.05, "billing": 0.9, "shipping": 0.05}, "confidence": 0.93},
    "angry": {"type": "noul", "noul": 0.81}
  },
  "usage": {"input_tokens": 120, "output_tokens": 0}
}`

func TestFromSystemOne_OrdersLikeRequest(t *testing.T) {
	req := mustRequest(t, openAIRequest)
	resp, err := ParseResponse([]byte(systemOneResponse), req)
	require.NoError(t, err)

	assert.Equal(t, "jev-1.13.0", resp.Model)
	assert.Equal(t, "decision", resp.Object)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 120, resp.Usage.InputTokens)

	out, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{
	  "object": "decision",
	  "model": "jev-1.13.0",
	  "answers": [
	    {"type": "predicate", "name": "angry", "probability": 0.81},
	    {"type": "choice", "name": "topic", "choice": "billing", "confidence": 0.93,
	     "probabilities": [{"value": "shipping", "probability": 0.05}, {"value": "billing", "probability": 0.9}, {"value": "other", "probability": 0.05}]},
	    {"type": "score", "name": "urgency", "score": 1.4, "confidence": 0.7,
	     "probabilities": [{"value": 0, "label": "low", "probability": 0.2}, {"value": 1, "label": "medium", "probability": 0.2}, {"value": 2, "label": "high", "probability": 0.6}]}
	  ],
	  "usage": {"input_tokens": 120, "output_tokens": 0}
	}`, string(out))
}

func TestFromSystemOne_MissingAndMismatchedAnswersAreRefusals(t *testing.T) {
	req := mustRequest(t, openAIRequest)
	resp, err := ParseResponse([]byte(`{"answers": {"angry": {"type": "choice", "choice": "x"}, "topic": {"type": "choice"}}}`), req)
	require.NoError(t, err)
	require.Len(t, resp.Answers, 3)
	for _, a := range resp.Answers {
		assert.Equal(t, TypeRefusal, a.Type, a.Name)
	}
}

func TestFromSystemOne_ScoreProbabilitiesKeyedByIndexOrText(t *testing.T) {
	req := mustRequest(t, openAIRequest)
	for _, probs := range []string{`{"0": 0.1, "1": 0.3, "2": 0.6}`, `{"low": 0.1, "medium: this week": 0.3, "high: today": 0.6}`} {
		resp, err := ParseResponse([]byte(`{"answers": {"urgency": {"type": "score", "score": 1.5, "probabilities": `+probs+`}}}`), req)
		require.NoError(t, err)
		score := resp.Answers[2]
		require.Equal(t, TypeScore, score.Type)
		require.Len(t, score.Probabilities, 3, probs)
		assert.Equal(t, 0.6, score.Probabilities[2].Probability)
		assert.Equal(t, "high", score.Probabilities[2].Label)
	}
}

func TestParseResponse_OpenAIPassesThrough(t *testing.T) {
	body := `{"id":"dec_1","object":"decision","model":"gpt-6-luna","answers":[{"type":"predicate","name":"a","probability":0.2},{"type":"refusal","name":"b"}],"usage":{"input_tokens":42,"output_tokens":0}}`
	resp, err := ParseResponse([]byte(body), nil)
	require.NoError(t, err)
	assert.Equal(t, "dec_1", resp.ID)
	require.Len(t, resp.Answers, 2)
	assert.Equal(t, TypeRefusal, resp.Answers[1].Type)

	_, err = ParseResponse([]byte(`{"choices":[]}`), nil)
	assert.Error(t, err)
}

func TestUsageFromBody_BothSpellings(t *testing.T) {
	u, ok := UsageFromBody([]byte(`{"answers":[],"usage":{"input_tokens":10,"output_tokens":2}}`))
	require.True(t, ok)
	assert.Equal(t, Usage{InputTokens: 10, OutputTokens: 2}, u)

	u, ok = UsageFromBody([]byte(`{"answers":[],"usage":{"prompt_tokens":7,"completion_tokens":0,"total_tokens":7}}`))
	require.True(t, ok)
	assert.Equal(t, Usage{InputTokens: 7, TotalTokens: 7}, u)

	_, ok = UsageFromBody([]byte(`{"answers":[]}`))
	assert.False(t, ok)
}

func TestSummarizeAndRender(t *testing.T) {
	s := Summarize([]byte(openAIRequest), FormatOpenAI, []byte(`{"answers":{"angry":{"type":"noul","noul":0.5},"topic":{"type":"choice","choice":"billing"}}}`))
	assert.Equal(t, 3, s.Questions)
	assert.Equal(t, 2, s.Answers)
	assert.Equal(t, 0, s.Refusals) // without the request, missing answers are not known
	assert.Equal(t, map[string]int{TypePredicate: 1, TypeChoice: 1}, s.ByType)

	soReq, err := ToSystemOne(mustRequest(t, openAIRequest))
	require.NoError(t, err)
	soBody, _ := json.Marshal(soReq)
	assert.Equal(t, 3, Summarize(soBody, FormatSystemOne, nil).Questions)

	resp, err := ParseResponse([]byte(systemOneResponse), mustRequest(t, openAIRequest))
	require.NoError(t, err)
	assert.Equal(t, "angry: predicate p=0.81\ntopic: choice=billing (confidence 0.93)\nurgency: score=1.4 (confidence 0.7)", RenderAnswers(resp))
}

func TestToSystemOne_NamesAndValuesAreEscapedNotMangled(t *testing.T) {
	req := mustRequest(t, `{"model":"m","input":"x","questions":[
	  {"type":"choice","name":"weird \"name\" <&>","instructions":"pick","choices":[{"value":"a\"b"},{"value":"ü <tag>"}]}]}`)
	so, err := ToSystemOne(req)
	require.NoError(t, err)
	body, err := json.Marshal(so)
	require.NoError(t, err)
	assert.True(t, json.Valid(body), string(body))

	var back SystemOneRequest
	require.NoError(t, json.Unmarshal(body, &back))
	require.Len(t, back.Questions, 1)
	assert.Equal(t, `weird "name" <&>`, back.Questions[0].Name)
	assert.Equal(t, `a"b`, ToOpenAIRequest(&back).Questions[0].Choices[0].Value)
}

func TestParseRequest_Malformed(t *testing.T) {
	_, err := ParseRequest([]byte(`{"model":`), FormatOpenAI)
	assert.Error(t, err)
	_, err = ParseRequest([]byte(`{"model":`), FormatSystemOne)
	assert.Error(t, err)
	_, err = ParseRequest([]byte(`{"model":"m","state":"x","questions":[1]}`), FormatSystemOne)
	assert.Error(t, err, "System One questions are an object")

	req, err := ParseRequest([]byte(`{"model":"m","state":"x","questions":null}`), FormatSystemOne)
	require.NoError(t, err)
	assert.Empty(t, req.Questions)
	assert.Equal(t, 0, Summarize([]byte(`{"model":"m","state":"x"}`), FormatSystemOne, nil).Questions)
}

func TestInputText_Shapes(t *testing.T) {
	text, images, err := InputText(json.RawMessage(`[{"role":"system","content":"rules"},{"role":"user","content":[{"type":"input_image","image_url":"data:x"}]}]`))
	require.NoError(t, err)
	assert.Equal(t, "rules", text)
	assert.Equal(t, 1, images)

	_, _, err = InputText(json.RawMessage(`{"not":"valid"}`))
	assert.Error(t, err)
	_, _, err = InputText(json.RawMessage(`[{"role":"user","content":7}]`))
	assert.Error(t, err)

	// Image-only input has no text for a System One model.
	_, err = ToSystemOne(mustRequest(t, `{"model":"m","input":[{"role":"user","content":""}],"questions":[{"type":"predicate","name":"q"}]}`))
	assert.Error(t, err)
}

func TestFromSystemOne_UnknownAnswerTypeIsARefusal(t *testing.T) {
	resp, err := ParseResponse([]byte(`{"answers":{"x":{"type":"mystery","value":1}}}`), nil)
	require.NoError(t, err)
	require.Len(t, resp.Answers, 1)
	assert.Equal(t, TypeRefusal, resp.Answers[0].Type)
}

func TestFormatForPath(t *testing.T) {
	cases := map[string]Format{
		"/llm/rest/openai/v1/decisions":        FormatOpenAI,
		"/v1/decisions/":                       FormatOpenAI,
		"/llm/call/jev/v1/systemone":           FormatSystemOne,
		"/llm/rest/openai/v1/chat/completions": "",
	}
	for path, want := range cases {
		got, ok := FormatForPath(path)
		assert.Equal(t, want != "", ok, path)
		assert.Equal(t, want, got, path)
	}
}
