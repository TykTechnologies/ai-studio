// Package decisions holds the wire types for decision models ("System One"
// models): models that answer typed questions about a piece of evidence with
// calibrated probabilities instead of generating text.
//
// Two wire formats are understood:
//
//   - OpenAI Decisions (POST /v1/decisions): the canonical client contract the
//     gateway exposes on its unified /v1 ingress. Questions are an ordered
//     array; answers come back as an array in the same order.
//   - System One (POST /v1/systemone): TypeSafe Jev and wire-compatible
//     self-hosts (llama.cpp serving Kev, OpenJev, Laya, ...). Questions and
//     answers are maps keyed by question name.
//
// The package is a dependency-free leaf so the proxy, the vendors, the plugin
// SDK and the microgateway can all share one parser and one translator.
package decisions

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// Upstream endpoints, relative to a vendor's API base.
const (
	OpenAIEndpoint    = "/v1/decisions"
	SystemOneEndpoint = "/v1/systemone"
)

// Question and answer types of the OpenAI (canonical) format.
const (
	TypePredicate = "predicate"
	TypeChoice    = "choice"
	TypeScore     = "score"
	// TypeRefusal marks an answer the model declined to give.
	TypeRefusal = "refusal"
)

// Question types of the System One format. Choice and score share their
// names with the OpenAI format; noul is System One's predicate.
const (
	SystemOneTypeNoul   = "noul"
	SystemOneTypeChoice = "choice"
	SystemOneTypeScore  = "score"
)

// Format names the wire format of a decision request or response body.
type Format string

const (
	FormatOpenAI    Format = "openai"
	FormatSystemOne Format = "systemone"
)

// KindDecision is the request kind (models.RequestKind, the plugin context's
// request_kind) of decision-model traffic. Generative traffic has kind "".
const KindDecision = "decision"

// KindForPath is the request kind of an upstream request path: KindDecision
// for a decision endpoint, "" otherwise.
func KindForPath(path string) string {
	if IsDecisionPath(path) {
		return KindDecision
	}
	return ""
}

// IsDecisionPath reports whether an upstream request path targets a decision
// endpoint in either format. The path may carry any prefix (/llm/rest/{slug},
// an Azure deployment path, ...); only the trailing endpoint is matched.
func IsDecisionPath(path string) bool {
	_, ok := FormatForPath(path)
	return ok
}

// FormatForPath reports which decision wire format an upstream path speaks.
func FormatForPath(path string) (Format, bool) {
	p := strings.TrimRight(strings.ToLower(path), "/")
	switch {
	case strings.HasSuffix(p, "/decisions"):
		return FormatOpenAI, true
	case strings.HasSuffix(p, "/systemone"):
		return FormatSystemOne, true
	}
	return "", false
}

// Request is an OpenAI-format decision request.
type Request struct {
	Model string `json:"model"`
	// Input is the shared evidence: a JSON string, or an array of user
	// messages whose content is a string or input_text/input_image parts.
	Input     json.RawMessage `json:"input,omitempty"`
	Questions []Question      `json:"questions"`
}

// Question is one OpenAI-format question.
type Question struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions,omitempty"`
	Choices      []Choice `json:"choices,omitempty"`
	// Levels are ordered lowest to highest.
	Levels []Level `json:"levels,omitempty"`
}

// Choice is one option of a choice question.
type Choice struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// Level is one rung of a score question.
type Level struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Response is an OpenAI-format decision response.
type Response struct {
	ID      string   `json:"id,omitempty"`
	Object  string   `json:"object,omitempty"`
	Model   string   `json:"model,omitempty"`
	Answers []Answer `json:"answers"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Answer is one OpenAI-format answer. Which fields are set depends on Type.
type Answer struct {
	Type string `json:"type"`
	Name string `json:"name"`
	// Probability is the predicate's probability that the condition holds.
	Probability *float64 `json:"probability,omitempty"`
	// Choice is the selected option of a choice question.
	Choice string `json:"choice,omitempty"`
	// Score is the probability-weighted average level index of a score question.
	Score         *float64            `json:"score,omitempty"`
	Probabilities []AnswerProbability `json:"probabilities,omitempty"`
	Confidence    *float64            `json:"confidence,omitempty"`
}

// AnswerProbability is one entry of a choice or score distribution. For a
// choice Value is the option string; for a score it is the level index and
// Label carries the level's label.
type AnswerProbability struct {
	Value       any     `json:"value"`
	Label       string  `json:"label,omitempty"`
	Probability float64 `json:"probability"`
}

// Usage is the token usage of a decision call. It accepts the
// input_tokens/output_tokens spelling (System One, OpenAI Responses-style)
// and the prompt_tokens/completion_tokens spelling (OpenAI chat-style).
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens,omitempty"`
	// InputTokensDetails is OpenAI's prompt-cache breakdown (seen on the live
	// API: cached_tokens and cache_write_tokens, both included in
	// InputTokens). System One reports none.
	InputTokensDetails *InputTokensDetails `json:"input_tokens_details,omitempty"`
}

// InputTokensDetails is the cache share of a call's input tokens.
type InputTokensDetails struct {
	CachedTokens     int `json:"cached_tokens"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

// UnmarshalJSON accepts either usage spelling.
func (u *Usage) UnmarshalJSON(data []byte) error {
	var raw struct {
		InputTokens        *int                `json:"input_tokens"`
		OutputTokens       *int                `json:"output_tokens"`
		PromptTokens       *int                `json:"prompt_tokens"`
		CompletionTokens   *int                `json:"completion_tokens"`
		TotalTokens        int                 `json:"total_tokens"`
		InputTokensDetails *InputTokensDetails `json:"input_tokens_details"`
		PromptTokensDetail *InputTokensDetails `json:"prompt_tokens_details"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = Usage{TotalTokens: raw.TotalTokens, InputTokensDetails: raw.InputTokensDetails}
	if u.InputTokensDetails == nil {
		u.InputTokensDetails = raw.PromptTokensDetail
	}
	switch {
	case raw.InputTokens != nil:
		u.InputTokens = *raw.InputTokens
	case raw.PromptTokens != nil:
		u.InputTokens = *raw.PromptTokens
	}
	switch {
	case raw.OutputTokens != nil:
		u.OutputTokens = *raw.OutputTokens
	case raw.CompletionTokens != nil:
		u.OutputTokens = *raw.CompletionTokens
	}
	return nil
}

// SystemOneRequest is a System One decision request. Questions keep their
// insertion order through OrderedQuestions: System One models are sensitive
// to option order, and a Go map would sort it.
type SystemOneRequest struct {
	Model     string
	State     json.RawMessage
	Questions OrderedQuestions
}

// SystemOneQuestion is one System One question. Instructions may be a string,
// an object or an array; Criteria depends on Type (an option->description
// object for choice, an array of levels for score, optional for noul).
type SystemOneQuestion struct {
	Type         string          `json:"type"`
	Instructions json.RawMessage `json:"instructions,omitempty"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}

// NamedQuestion is a System One question with its map key.
type NamedQuestion struct {
	Name     string
	Question SystemOneQuestion
}

// OrderedQuestions is the System One questions map in wire order.
type OrderedQuestions []NamedQuestion

// MarshalJSON writes the request with questions as an ordered object.
func (r SystemOneRequest) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"model":`)
	if err := writeJSON(&buf, r.Model); err != nil {
		return nil, err
	}
	buf.WriteString(`,"state":`)
	if len(r.State) == 0 {
		buf.WriteString(`""`)
	} else {
		buf.Write(r.State)
	}
	buf.WriteString(`,"questions":{`)
	for i, q := range r.Questions {
		if i > 0 {
			buf.WriteByte(',')
		}
		if err := writeJSON(&buf, q.Name); err != nil {
			return nil, err
		}
		buf.WriteByte(':')
		if err := writeJSON(&buf, q.Question); err != nil {
			return nil, err
		}
	}
	buf.WriteString(`}}`)
	return buf.Bytes(), nil
}

// UnmarshalJSON reads a System One request, keeping question order.
func (r *SystemOneRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		Model     string          `json:"model"`
		State     json.RawMessage `json:"state"`
		Questions json.RawMessage `json:"questions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Model = raw.Model
	r.State = raw.State
	r.Questions = nil
	if len(raw.Questions) == 0 || string(raw.Questions) == "null" {
		return nil
	}
	entries, err := orderedObject(raw.Questions)
	if err != nil {
		return err
	}
	for _, e := range entries {
		var q SystemOneQuestion
		if err := json.Unmarshal(e.Value, &q); err != nil {
			return err
		}
		r.Questions = append(r.Questions, NamedQuestion{Name: e.Key, Question: q})
	}
	return nil
}

// SystemOneResponse is a System One decision response.
type SystemOneResponse struct {
	Model   string                     `json:"model,omitempty"`
	Answers map[string]SystemOneAnswer `json:"answers"`
	Usage   *Usage                     `json:"usage,omitempty"`
}

// SystemOneAnswer is one System One answer. Probabilities is an
// option->probability object for choice, and an array or index-keyed object
// for score.
type SystemOneAnswer struct {
	Type          string          `json:"type"`
	Noul          *float64        `json:"noul,omitempty"`
	Choice        string          `json:"choice,omitempty"`
	Score         *float64        `json:"score,omitempty"`
	Legend        json.RawMessage `json:"legend,omitempty"`
	Probabilities json.RawMessage `json:"probabilities,omitempty"`
	Confidence    *float64        `json:"confidence,omitempty"`
}

func writeJSON(buf *bytes.Buffer, v any) error {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	// Encoder.Encode appends a newline.
	buf.Truncate(buf.Len() - 1)
	return nil
}

type objectEntry struct {
	Key   string
	Value json.RawMessage
}

// orderedObject decodes a JSON object into its entries in wire order.
func orderedObject(data []byte) ([]objectEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("expected a JSON object")
	}
	var out []objectEntry
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, objectEntry{Key: key, Value: v})
	}
	return out, nil
}
