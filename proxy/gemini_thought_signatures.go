package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms/googleai"
)

// Gemini 3 thought signatures on the /ai and /v1 bridges.
//
// A Gemini 3 model returns a thoughtSignature on its functionCall parts and
// refuses the next turn (400) when those parts come back without it. The Google
// AI client the bridge drives has no field for it and drops it, so the loopback
// transport carries it past the client: on the response it moves each
// functionCall part's thoughtSignature into the call's args under
// googleai.ThoughtSignatureArg, where the langchaingo driver moves it into the
// tool call's ID (an OpenAI-shaped client echoes tool calls back unchanged); on
// the next request the driver puts it back in the args and the transport
// restores it on the part. When a turn's first call has no signature to
// restore, it is sent with Google's placeholder instead, so the request is
// never refused for it. The reserved args key never reaches the caller,
// /llm/ or the API.

const thoughtSignatureField = "thoughtSignature"

var (
	thoughtSignatureFieldBytes = []byte(`"` + thoughtSignatureField + `"`)
	functionCallBytes          = []byte(`"functionCall"`)
)

// placeholderThoughtSignature is Google's documented stand-in for a signature
// that cannot be recovered: it lets the request through, without the
// reasoning context the real one carries.
const placeholderThoughtSignature = "skip_thought_signature_validator"

// restoreThoughtSignatures puts the thought signatures back on a Gemini
// generateContent request body's role:model functionCall parts: a call's
// args[ThoughtSignatureArg] (decoded from its tool call ID) moves onto its
// part. Gemini signs the first call of each model turn, so when that one has
// no signature to restore (a client rewrote the ID, the history predates
// this, the ID was malformed) it gets the placeholder (not on Gemini 1.x and
// 2.x, see placeholderAllowed); other calls are left as they are. A body
// without function calls is returned as it is.
func restoreThoughtSignatures(body []byte) []byte {
	if !bytes.Contains(body, functionCallBytes) {
		return body
	}
	var req map[string]any
	if err := decodeJSONNumbers(body, &req); err != nil {
		return body
	}
	changed := false
	model, _ := req["model"].(string)
	placeholder := placeholderAllowed(model)
	contents, _ := req["contents"].([]any)
	for _, c := range contents {
		content, _ := c.(map[string]any)
		parts, _ := content["parts"].([]any)
		isModel := content["role"] == "model"
		first := true
		for _, p := range parts {
			part, _ := p.(map[string]any)
			fc, ok := part["functionCall"].(map[string]any)
			if !ok {
				continue
			}
			args, _ := fc["args"].(map[string]any)
			sig, hasSig := args[googleai.ThoughtSignatureArg].(string)
			if _, present := args[googleai.ThoughtSignatureArg]; present {
				delete(args, googleai.ThoughtSignatureArg) // never sent on, whatever the role
				changed = true
			}
			if !isModel {
				continue
			}
			switch {
			case hasSig && sig != "":
				part[thoughtSignatureField] = sig
				changed = true
			case first && placeholder && part[thoughtSignatureField] == nil:
				part[thoughtSignatureField] = placeholderThoughtSignature
				changed = true
			}
			first = false
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

// carryThoughtSignatures moves each functionCall part's thoughtSignature into
// the call's args under ThoughtSignatureArg, in one generateContent response
// object. An object without a signature is returned as it is.
func carryThoughtSignatures(chunk []byte) []byte {
	if !bytes.Contains(chunk, thoughtSignatureFieldBytes) {
		return chunk
	}
	var resp map[string]any
	if err := decodeJSONNumbers(chunk, &resp); err != nil {
		return chunk
	}
	changed := false
	candidates, _ := resp["candidates"].([]any)
	for _, c := range candidates {
		candidate, _ := c.(map[string]any)
		content, _ := candidate["content"].(map[string]any)
		parts, _ := content["parts"].([]any)
		for _, p := range parts {
			part, _ := p.(map[string]any)
			fc, ok := part["functionCall"].(map[string]any)
			sig, hasSig := part[thoughtSignatureField].(string)
			if !ok || !hasSig || sig == "" {
				continue
			}
			args, _ := fc["args"].(map[string]any)
			if args == nil {
				args = map[string]any{}
				fc["args"] = args
			}
			args[googleai.ThoughtSignatureArg] = sig
			delete(part, thoughtSignatureField)
			changed = true
		}
	}
	if !changed {
		return chunk
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return chunk
	}
	return out
}

// placeholderAllowed reports whether a model gets the placeholder for a
// signature it cannot have back. Gemini 1.x and 2.x models do not require
// signatures and their acceptance of the placeholder is unverified, so they
// get none; real signatures are restored for every model.
func placeholderAllowed(model string) bool {
	model = strings.TrimPrefix(model, "models/")
	return !strings.HasPrefix(model, "gemini-1") && !strings.HasPrefix(model, "gemini-2")
}

func decodeJSONNumbers(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}

// withThoughtSignaturesRestored rewrites req's body with
// restoreThoughtSignatures.
func withThoughtSignaturesRestored(req *http.Request) error {
	if req.Body == nil || req.Body == http.NoBody {
		return nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return err
	}
	body = restoreThoughtSignatures(body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

// signatureCarrier is a response body that applies carryThoughtSignatures to
// every top-level JSON object of the stream as it completes: the elements of a
// streamed JSON array ([{...},{...}]) or a single object. Everything between
// the objects passes through as it is, and an object is passed on as soon as
// its closing brace arrives, so a stream is not held back.
type signatureCarrier struct {
	src  *bufio.Reader
	body io.Closer
	out  bytes.Buffer // bytes ready for the reader
	obj  bytes.Buffer // the top-level object being read
	// depth counts open brackets and braces; objDepth is the depth at which
	// the current top-level object started (-1: none).
	depth, objDepth int
	inString, esc   bool
	err             error
}

func newSignatureCarrier(body io.ReadCloser) *signatureCarrier {
	return &signatureCarrier{src: bufio.NewReader(body), body: body, objDepth: -1}
}

func (s *signatureCarrier) Read(p []byte) (int, error) {
	for s.out.Len() == 0 && s.err == nil {
		s.fill()
	}
	if s.out.Len() > 0 {
		return s.out.Read(p)
	}
	return 0, s.err
}

// fill consumes what the source has buffered (at least one byte).
func (s *signatureCarrier) fill() {
	b, err := s.src.ReadByte()
	if err != nil {
		if s.obj.Len() > 0 { // a truncated object: pass on what came
			s.out.Write(s.obj.Bytes())
			s.obj.Reset()
		}
		s.err = err
		return
	}
	s.step(b)
	for n := s.src.Buffered(); n > 0; n-- {
		b, _ = s.src.ReadByte()
		s.step(b)
	}
}

func (s *signatureCarrier) step(b byte) {
	if s.objDepth < 0 {
		// Outside any top-level object: an array's brackets and separators.
		switch {
		case b == '{' && s.depth <= 1:
			s.objDepth = s.depth
			s.depth++
			s.obj.WriteByte(b)
			return
		case b == '[':
			s.depth++
		case b == ']' && s.depth > 0:
			s.depth--
		}
		s.out.WriteByte(b)
		return
	}
	s.obj.WriteByte(b)
	switch {
	case s.esc:
		s.esc = false
	case s.inString:
		if b == '\\' {
			s.esc = true
		} else if b == '"' {
			s.inString = false
		}
	case b == '"':
		s.inString = true
	case b == '{' || b == '[':
		s.depth++
	case b == '}' || b == ']':
		s.depth--
		if s.depth == s.objDepth {
			s.out.Write(carryThoughtSignatures(s.obj.Bytes()))
			s.obj.Reset()
			s.objDepth = -1
		}
	}
}

func (s *signatureCarrier) Close() error { return s.body.Close() }
