package decisions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// System One limits (TypeSafe API reference).
const (
	MaxSystemOneChoices = 255
	MaxSystemOneLevels  = 10
)

// ErrImagesUnsupported is returned when a request carrying image input is
// translated for an upstream that only accepts text state.
var ErrImagesUnsupported = errors.New("image input is not supported by this decision model; send text input only")

// ValidationError is a client error in a decision request. Param names the
// offending field in the OpenAI error envelope.
type ValidationError struct {
	Param   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(param, format string, args ...any) error {
	return &ValidationError{Param: param, Message: fmt.Sprintf(format, args...)}
}

// Validate checks an OpenAI-format request before it is forwarded to target.
// It enforces what the gateway relies on (named, typed questions it can count
// and translate) plus the target's documented limits; anything else is left
// to the upstream to judge.
func Validate(req *Request, target Format) error {
	if req == nil {
		return invalid("", "request body is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return invalid("model", "'model' is required")
	}
	if len(bytes.TrimSpace(req.Input)) == 0 || string(bytes.TrimSpace(req.Input)) == "null" {
		return invalid("input", "'input' is required")
	}
	if len(req.Questions) == 0 {
		return invalid("questions", "'questions' must contain at least one question")
	}
	seen := make(map[string]bool, len(req.Questions))
	for i, q := range req.Questions {
		param := fmt.Sprintf("questions[%d]", i)
		if strings.TrimSpace(q.Name) == "" {
			return invalid(param+".name", "question %d has no 'name'", i)
		}
		if seen[q.Name] {
			return invalid(param+".name", "question name '%s' is used more than once", q.Name)
		}
		seen[q.Name] = true
		switch q.Type {
		case TypePredicate:
		case TypeChoice:
			if len(q.Choices) < 2 {
				return invalid(param+".choices", "choice question '%s' needs at least 2 choices", q.Name)
			}
			if target == FormatSystemOne && len(q.Choices) > MaxSystemOneChoices {
				return invalid(param+".choices", "choice question '%s' has %d choices; this model allows at most %d", q.Name, len(q.Choices), MaxSystemOneChoices)
			}
			values := make(map[string]bool, len(q.Choices))
			for _, c := range q.Choices {
				if c.Value == "" {
					return invalid(param+".choices", "choice question '%s' has a choice with no 'value'", q.Name)
				}
				if values[c.Value] {
					return invalid(param+".choices", "choice question '%s' repeats the value '%s'", q.Name, c.Value)
				}
				values[c.Value] = true
			}
		case TypeScore:
			if len(q.Levels) < 2 {
				return invalid(param+".levels", "score question '%s' needs at least 2 levels", q.Name)
			}
			if target == FormatSystemOne && len(q.Levels) > MaxSystemOneLevels {
				return invalid(param+".levels", "score question '%s' has %d levels; this model allows at most %d", q.Name, len(q.Levels), MaxSystemOneLevels)
			}
			for _, l := range q.Levels {
				if l.Label == "" && l.Description == "" {
					return invalid(param+".levels", "score question '%s' has an empty level", q.Name)
				}
			}
		default:
			return invalid(param+".type", "question '%s' has unsupported type '%s' (expected predicate, choice or score)", q.Name, q.Type)
		}
	}
	return nil
}

// InputText flattens an OpenAI-format input into plain text and counts its
// image parts. A string input is returned as is; message inputs have their
// text parts joined by blank lines.
func InputText(input json.RawMessage) (string, int, error) {
	input = bytes.TrimSpace(input)
	if len(input) == 0 || string(input) == "null" {
		return "", 0, nil
	}
	if input[0] == '"' {
		var s string
		if err := json.Unmarshal(input, &s); err != nil {
			return "", 0, err
		}
		return s, 0, nil
	}
	var msgs []inputMessage
	if err := json.Unmarshal(input, &msgs); err != nil {
		return "", 0, fmt.Errorf("'input' must be a string or an array of messages: %w", err)
	}
	var texts []string
	images := 0
	for _, m := range msgs {
		t, n, err := m.text()
		if err != nil {
			return "", 0, err
		}
		images += n
		if t != "" {
			texts = append(texts, t)
		}
	}
	return strings.Join(texts, "\n\n"), images, nil
}

type inputMessage struct {
	Role    string          `json:"role,omitempty"`
	Content json.RawMessage `json:"content"`
}

type inputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL any    `json:"image_url,omitempty"`
}

func (m inputMessage) text() (string, int, error) {
	c := bytes.TrimSpace(m.Content)
	if len(c) == 0 || string(c) == "null" {
		return "", 0, nil
	}
	if c[0] == '"' {
		var s string
		err := json.Unmarshal(c, &s)
		return s, 0, err
	}
	var parts []inputPart
	if err := json.Unmarshal(c, &parts); err != nil {
		return "", 0, fmt.Errorf("message content must be a string or an array of parts: %w", err)
	}
	var texts []string
	images := 0
	for _, p := range parts {
		switch p.Type {
		case "input_text", "text":
			if p.Text != "" {
				texts = append(texts, p.Text)
			}
		case "input_image", "image_url", "image":
			images++
		}
	}
	return strings.Join(texts, "\n"), images, nil
}

// ToSystemOne translates an OpenAI-format request into a System One request.
// Question order and option order are preserved.
func ToSystemOne(req *Request) (*SystemOneRequest, error) {
	if err := Validate(req, FormatSystemOne); err != nil {
		return nil, err
	}
	state, err := stateFromInput(req.Input)
	if err != nil {
		return nil, err
	}
	out := &SystemOneRequest{Model: req.Model, State: state}
	for _, q := range req.Questions {
		instr, _ := json.Marshal(q.Instructions)
		sq := SystemOneQuestion{Instructions: instr}
		switch q.Type {
		case TypePredicate:
			sq.Type = SystemOneTypeNoul
		case TypeChoice:
			sq.Type = SystemOneTypeChoice
			var buf bytes.Buffer
			buf.WriteByte('{')
			for i, c := range q.Choices {
				if i > 0 {
					buf.WriteByte(',')
				}
				desc := c.Description
				if desc == "" {
					desc = c.Value
				}
				if err := writeJSON(&buf, c.Value); err != nil {
					return nil, err
				}
				buf.WriteByte(':')
				if err := writeJSON(&buf, desc); err != nil {
					return nil, err
				}
			}
			buf.WriteByte('}')
			sq.Criteria = buf.Bytes()
		case TypeScore:
			sq.Type = SystemOneTypeScore
			levels := make([]string, len(q.Levels))
			for i, l := range q.Levels {
				levels[i] = levelText(l)
			}
			sq.Criteria, _ = json.Marshal(levels)
		}
		out.Questions = append(out.Questions, NamedQuestion{Name: q.Name, Question: sq})
	}
	return out, nil
}

// levelText is the System One criterion for an OpenAI score level.
func levelText(l Level) string {
	switch {
	case l.Description == "":
		return l.Label
	case l.Label == "":
		return l.Description
	default:
		return l.Label + ": " + l.Description
	}
}

// stateFromInput turns an OpenAI-format input into System One state: a string
// input passes through unchanged, message inputs become their joined text.
func stateFromInput(input json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		return trimmed, nil
	}
	text, images, err := InputText(trimmed)
	if err != nil {
		return nil, invalid("input", "%s", err.Error())
	}
	if images > 0 {
		return nil, &ValidationError{Param: "input", Message: ErrImagesUnsupported.Error()}
	}
	if strings.TrimSpace(text) == "" {
		return nil, invalid("input", "'input' has no text content")
	}
	return json.Marshal(text)
}

// FromSystemOne translates a System One response into an OpenAI-format
// response, returning answers in the order of req's questions. A question
// with no answer, or an answer of an unexpected type, becomes a refusal.
func FromSystemOne(so *SystemOneResponse, req *Request) *Response {
	out := &Response{Object: "decision", Model: so.Model, Usage: so.Usage}
	if req == nil {
		names := make([]string, 0, len(so.Answers))
		for name := range so.Answers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out.Answers = append(out.Answers, answerFromSystemOne(name, so.Answers[name], nil))
		}
		return out
	}
	out.Answers = make([]Answer, 0, len(req.Questions))
	for i := range req.Questions {
		q := &req.Questions[i]
		a, ok := so.Answers[q.Name]
		if !ok {
			out.Answers = append(out.Answers, Answer{Type: TypeRefusal, Name: q.Name})
			continue
		}
		out.Answers = append(out.Answers, answerFromSystemOne(q.Name, a, q))
	}
	return out
}

func answerFromSystemOne(name string, a SystemOneAnswer, q *Question) Answer {
	refusal := Answer{Type: TypeRefusal, Name: name}
	typ := a.Type
	if q != nil {
		// The request is authoritative; an answer of another type is unusable.
		want := q.Type
		if want == TypePredicate {
			want = SystemOneTypeNoul
		}
		if typ != "" && typ != want {
			return refusal
		}
		typ = want
	}
	switch typ {
	case SystemOneTypeNoul, TypePredicate:
		if a.Noul == nil {
			return refusal
		}
		return Answer{Type: TypePredicate, Name: name, Probability: a.Noul}
	case SystemOneTypeChoice:
		if a.Choice == "" {
			return refusal
		}
		byKey, _ := parseProbabilities(a.Probabilities)
		ans := Answer{Type: TypeChoice, Name: name, Choice: a.Choice, Confidence: a.Confidence}
		if q != nil {
			for _, c := range q.Choices {
				if p, ok := byKey[c.Value]; ok {
					ans.Probabilities = append(ans.Probabilities, AnswerProbability{Value: c.Value, Probability: p})
				}
			}
		} else {
			for _, k := range sortedKeys(byKey) {
				ans.Probabilities = append(ans.Probabilities, AnswerProbability{Value: k, Probability: byKey[k]})
			}
		}
		return ans
	case SystemOneTypeScore:
		if a.Score == nil {
			return refusal
		}
		byKey, byIndex := parseProbabilities(a.Probabilities)
		ans := Answer{Type: TypeScore, Name: name, Score: a.Score, Confidence: a.Confidence}
		if q != nil {
			for i, l := range q.Levels {
				p, ok := levelProbability(i, l, byKey, byIndex)
				if !ok {
					continue
				}
				ans.Probabilities = append(ans.Probabilities, AnswerProbability{Value: i, Label: l.Label, Probability: p})
			}
		} else {
			legend := parseLegend(a.Legend)
			for i, p := range byIndex {
				ans.Probabilities = append(ans.Probabilities, AnswerProbability{Value: i, Label: legend[strconv.Itoa(i)], Probability: p})
			}
			if len(byIndex) == 0 {
				for _, k := range sortedKeys(byKey) {
					idx, err := strconv.Atoi(k)
					if err != nil {
						continue
					}
					ans.Probabilities = append(ans.Probabilities, AnswerProbability{Value: idx, Label: legend[k], Probability: byKey[k]})
				}
			}
		}
		return ans
	}
	return refusal
}

func levelProbability(i int, l Level, byKey map[string]float64, byIndex []float64) (float64, bool) {
	if i < len(byIndex) {
		return byIndex[i], true
	}
	for _, k := range []string{strconv.Itoa(i), levelText(l), l.Label, l.Description} {
		if k == "" {
			continue
		}
		if p, ok := byKey[k]; ok {
			return p, true
		}
	}
	return 0, false
}

// parseProbabilities reads a probabilities field that is either an object
// (option or level -> probability) or an array of probabilities.
func parseProbabilities(raw json.RawMessage) (map[string]float64, []float64) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	switch raw[0] {
	case '{':
		var m map[string]float64
		if json.Unmarshal(raw, &m) == nil {
			return m, nil
		}
	case '[':
		var arr []float64
		if json.Unmarshal(raw, &arr) == nil {
			return nil, arr
		}
	}
	return nil, nil
}

func parseLegend(raw json.RawMessage) map[string]string {
	raw = bytes.TrimSpace(raw)
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	if raw[0] == '[' {
		var arr []string
		if json.Unmarshal(raw, &arr) == nil {
			for i, s := range arr {
				out[strconv.Itoa(i)] = s
			}
		}
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ToOpenAIRequest converts a System One request into the OpenAI shape, so
// plugins and filters can reason about one schema. State becomes a string
// input (objects and arrays are carried as compact JSON text).
func ToOpenAIRequest(so *SystemOneRequest) *Request {
	out := &Request{Model: so.Model}
	state := bytes.TrimSpace(so.State)
	if len(state) > 0 && state[0] == '"' {
		out.Input = append(json.RawMessage(nil), state...)
	} else if len(state) > 0 {
		out.Input, _ = json.Marshal(rawText(state))
	}
	for _, nq := range so.Questions {
		q := Question{Name: nq.Name, Instructions: rawText(nq.Question.Instructions)}
		switch nq.Question.Type {
		case SystemOneTypeNoul:
			q.Type = TypePredicate
		case SystemOneTypeChoice:
			q.Type = TypeChoice
			if entries, err := orderedObject(nq.Question.Criteria); err == nil {
				for _, e := range entries {
					q.Choices = append(q.Choices, Choice{Value: e.Key, Description: rawText(e.Value)})
				}
			}
		case SystemOneTypeScore:
			q.Type = TypeScore
			var levels []json.RawMessage
			if json.Unmarshal(nq.Question.Criteria, &levels) == nil {
				for _, l := range levels {
					q.Levels = append(q.Levels, Level{Label: rawText(l)})
				}
			}
		default:
			q.Type = nq.Question.Type
		}
		out.Questions = append(out.Questions, q)
	}
	return out
}

// StateText renders System One state (or instructions) as text: a string as
// is, an object or array as compact JSON.
func StateText(raw json.RawMessage) string {
	return rawText(raw)
}

// rawText renders a JSON value as text: strings unquoted, anything else as
// compact JSON.
func rawText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if raw[0] == '"' && json.Unmarshal(raw, &s) == nil {
		return s
	}
	var buf bytes.Buffer
	if json.Compact(&buf, raw) == nil {
		return buf.String()
	}
	return string(raw)
}

// ParseRequest parses a request body in the given format into the OpenAI shape.
func ParseRequest(body []byte, format Format) (*Request, error) {
	switch format {
	case FormatSystemOne:
		var so SystemOneRequest
		if err := json.Unmarshal(body, &so); err != nil {
			return nil, err
		}
		return ToOpenAIRequest(&so), nil
	default:
		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}
		return &req, nil
	}
}

// DetectResponseFormat reports the wire format of a decision response body
// from the shape of its answers field (an array for OpenAI, an object for
// System One).
func DetectResponseFormat(body []byte) (Format, bool) {
	var probe struct {
		Answers json.RawMessage `json:"answers"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return "", false
	}
	a := bytes.TrimSpace(probe.Answers)
	if len(a) == 0 {
		return "", false
	}
	switch a[0] {
	case '[':
		return FormatOpenAI, true
	case '{':
		return FormatSystemOne, true
	}
	return "", false
}

// ParseResponse parses a response body in either format into the OpenAI
// shape. req, when given, orders System One answers like its questions.
func ParseResponse(body []byte, req *Request) (*Response, error) {
	format, ok := DetectResponseFormat(body)
	if !ok {
		return nil, errors.New("not a decision response: missing 'answers'")
	}
	if format == FormatSystemOne {
		var so SystemOneResponse
		if err := json.Unmarshal(body, &so); err != nil {
			return nil, err
		}
		return FromSystemOne(&so, req), nil
	}
	var resp Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UsageFromBody reads the usage block of a decision response in either format.
func UsageFromBody(body []byte) (Usage, bool) {
	var probe struct {
		Usage *Usage `json:"usage"`
	}
	if json.Unmarshal(body, &probe) != nil || probe.Usage == nil {
		return Usage{}, false
	}
	return *probe.Usage, true
}

// Summary counts what a decision exchange asked and answered.
type Summary struct {
	Questions int
	Answers   int
	Refusals  int
	// ByType counts answered questions per OpenAI answer type.
	ByType map[string]int
}

// Summarize counts the questions in a request body (either format) and the
// answers and refusals in a response body. Either body may be nil.
func Summarize(reqBody []byte, reqFormat Format, respBody []byte) Summary {
	s := Summary{ByType: map[string]int{}}
	if len(reqBody) > 0 {
		if req, err := ParseRequest(reqBody, reqFormat); err == nil {
			s.Questions = len(req.Questions)
		}
	}
	if len(respBody) > 0 {
		if resp, err := ParseResponse(respBody, nil); err == nil {
			for _, a := range resp.Answers {
				if a.Type == TypeRefusal {
					s.Refusals++
					continue
				}
				s.Answers++
				s.ByType[a.Type]++
			}
		}
	}
	return s
}

// RenderAnswers renders a response as compact text, one answer per line, for
// text-based response filters and logs.
func RenderAnswers(resp *Response) string {
	var b strings.Builder
	for i, a := range resp.Answers {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(a.Name)
		b.WriteString(": ")
		switch a.Type {
		case TypePredicate:
			b.WriteString("predicate p=" + formatFloat(a.Probability))
		case TypeChoice:
			b.WriteString("choice=" + a.Choice)
		case TypeScore:
			b.WriteString("score=" + formatFloat(a.Score))
		case TypeRefusal:
			b.WriteString("refused")
		default:
			b.WriteString(a.Type)
		}
		if a.Confidence != nil {
			b.WriteString(" (confidence " + formatFloat(a.Confidence) + ")")
		}
	}
	return b.String()
}

func formatFloat(f *float64) string {
	if f == nil {
		return "?"
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}
