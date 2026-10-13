package anthropicVendor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

// ResourceMessageBatch is the native resource type of a Message Batch.
const ResourceMessageBatch = "anthropic.message_batch"

// batchResultsRetention is how long Anthropic keeps a batch's results after
// the batch was created.
const batchResultsRetention = 29 * 24 * time.Hour

// batchIDCharset is the shape of an Anthropic batch ID (msgbatch_01Hk...).
var batchIDCharset = regexp.MustCompile(`^msgbatch_[A-Za-z0-9_-]{1,200}$`)

// nativeHeaderAllow is every header of the client request that reaches
// Anthropic on a native operation. Workspace and user-profile pins are left
// out: resources live in the LLM's vendor workspace, whichever the caller
// names.
var nativeHeaderAllow = []string{"anthropic-version", "anthropic-beta", "content-type", "accept", "x-request-id"}

// nativeQueryAllow is every query parameter that reaches Anthropic. The SDKs
// add ?beta=true on beta endpoints; the list cursors are ledger-served and
// never forwarded.
var nativeQueryAllow = []string{"beta"}

// NativeAPIVersion is the anthropic-version the gateway sends on calls it
// makes without a client request (the reconciler, a cancel after a failed
// ledger write) when the client did not name one.
const NativeAPIVersion = "2023-06-01"

func nativeDefaultHeader() http.Header {
	return http.Header{"Anthropic-Version": []string{NativeAPIVersion}}
}

var nativeSpec = nativeapi.Spec{
	Resources: []nativeapi.ResourceType{{
		Name:      ResourceMessageBatch,
		IDCharset: batchIDCharset,
		Async: &nativeapi.AsyncSpec{
			Poll: func(vendorID string) nativeapi.UpstreamReq {
				return nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/messages/batches/" + vendorID, Header: nativeDefaultHeader()}
			},
			State: batchState,
			LineUsage: func(line []byte) (nativeapi.Usage, bool, error) {
				v, err := viewResultLine(line)
				if err != nil || !v.Usage.Billed {
					return nativeapi.Usage{}, false, err
				}
				return v.Usage, true, nil
			},
			Cancel: func(vendorID string) nativeapi.UpstreamReq {
				return nativeapi.UpstreamReq{Method: http.MethodPost, Path: "/v1/messages/batches/" + vendorID + "/cancel", Header: nativeDefaultHeader()}
			},
			Results: func(vendorID string, obj []byte) (nativeapi.UpstreamReq, bool) {
				var b messageBatch
				if json.Unmarshal(obj, &b) != nil || b.ProcessingStatus != "ended" || b.ResultsURL == nil || *b.ResultsURL == "" {
					return nativeapi.UpstreamReq{}, false
				}
				return nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/messages/batches/" + vendorID + "/results", Header: nativeDefaultHeader()}, true
			},
		},
	}},
	Operations: []nativeapi.Operation{
		{
			Name:        "anthropic.message_batches.create",
			Method:      http.MethodPost,
			Path:        "/v1/messages/batches",
			Ledger:      nativeapi.LedgerCreate,
			Budget:      true,
			Admit:       &nativeapi.AdmitSpec{Split: splitBatch, Join: joinBatch},
			ResponseIDs: []nativeapi.BodyRef{{Path: "id", Resource: ResourceMessageBatch}},
			Transform:   rewriteResultsURL,
			Upstream:    nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow},
			ErrorShape:  NativeError,
		},
		{
			// Served from this gateway's ledger, not forwarded: the vendor
			// would list every App's batches in the workspace.
			Name:   "anthropic.message_batches.list",
			Method: http.MethodGet,
			Path:   "/v1/messages/batches",
			Ledger: nativeapi.LedgerList,
			// The listed resource type; the page's IDs are wrapped for it.
			ResponseIDs: []nativeapi.BodyRef{{Path: "data[].id", Resource: ResourceMessageBatch}},
			List: &nativeapi.ListSpec{
				Envelope:   listEnvelope,
				LimitParam: "limit", AfterParam: "after_id", BeforeParam: "before_id",
				DefaultLimit: 20, MaxLimit: 1000,
			},
			ErrorShape: NativeError,
		},
		{
			Name:         "anthropic.message_batches.retrieve",
			Method:       http.MethodGet,
			Path:         "/v1/messages/batches/{id}",
			PathResource: ResourceMessageBatch,
			Ledger:       nativeapi.LedgerRead,
			ResponseIDs:  []nativeapi.BodyRef{{Path: "id", Resource: ResourceMessageBatch}},
			Transform:    rewriteResultsURL,
			Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow},
			ErrorShape:   NativeError,
		},
		{
			Name:         "anthropic.message_batches.results",
			Method:       http.MethodGet,
			Path:         "/v1/messages/batches/{id}/results",
			PathResource: ResourceMessageBatch,
			Ledger:       nativeapi.LedgerRead,
			Results:      &nativeapi.ResultsSpec{View: viewResultLine, Blocked: blockedResultLine},
			Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow, LongRunning: true},
			ErrorShape:   NativeError,
		},
		{
			Name:         "anthropic.message_batches.cancel",
			Method:       http.MethodPost,
			Path:         "/v1/messages/batches/{id}/cancel",
			PathResource: ResourceMessageBatch,
			Ledger:       nativeapi.LedgerMutate,
			ResponseIDs:  []nativeapi.BodyRef{{Path: "id", Resource: ResourceMessageBatch}},
			Transform:    rewriteResultsURL,
			Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow},
			ErrorShape:   NativeError,
		},
		{
			Name:         "anthropic.message_batches.delete",
			Method:       http.MethodDelete,
			Path:         "/v1/messages/batches/{id}",
			PathResource: ResourceMessageBatch,
			Ledger:       nativeapi.LedgerDelete,
			ResponseIDs:  []nativeapi.BodyRef{{Path: "id", Resource: ResourceMessageBatch}},
			Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow},
			ErrorShape:   NativeError,
		},
		{
			// Claude Code counts tokens before most turns. The body is one
			// Messages request: its model is checked against the allow-list
			// and the request filters run on it, then it is forwarded and
			// logged without a chat record (counting is free).
			Name:       "anthropic.messages.count_tokens",
			Method:     http.MethodPost,
			Path:       "/v1/messages/count_tokens",
			Ledger:     nativeapi.LedgerNone,
			Budget:     true,
			Admit:      &nativeapi.AdmitSpec{Split: splitCountTokens, Join: joinSingle},
			Upstream:   nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, QueryAllow: nativeQueryAllow},
			ErrorShape: NativeError,
		},
	},
	ChainHooks: &nativeapi.ChainHooks{Hooks: []nativeapi.ChainHook{{
		// The synchronous Messages call: a body with duplicate keys at any
		// depth is refused before the model check and the filters read it.
		Name:       "anthropic.messages",
		Method:     http.MethodPost,
		Path:       "/v1/messages",
		CheckBody:  checkMessagesKeys,
		ErrorShape: NativeError,
	}}},
}

// NativeAPI implements nativeapi.Provider: Anthropic's Message Batches API on
// the native /llm routes.
func (v *Anthropic) NativeAPI() nativeapi.Spec { return nativeSpec }

// messageBatch is the part of an Anthropic Message Batch object the gateway
// reads.
type messageBatch struct {
	ID               string     `json:"id"`
	ProcessingStatus string     `json:"processing_status"`
	CreatedAt        *time.Time `json:"created_at"`
	EndedAt          *time.Time `json:"ended_at"`
	ResultsURL       *string    `json:"results_url"`
}

// batchState maps a batch object onto the coarse lifecycle the reconciler
// drives. Results are kept for 29 days from creation.
func batchState(obj []byte) (nativeapi.StateInfo, error) {
	var b messageBatch
	if err := json.Unmarshal(obj, &b); err != nil {
		return nativeapi.StateInfo{}, err
	}
	info := nativeapi.StateInfo{EndedAt: b.EndedAt}
	switch b.ProcessingStatus {
	case "ended":
		info.State = nativeapi.StateEnded
	case "canceling":
		info.State = nativeapi.StateCanceling
	default:
		info.State = nativeapi.StateInProgress
	}
	if b.CreatedAt != nil {
		exp := b.CreatedAt.Add(batchResultsRetention)
		info.ResultsExpireAt = &exp
	}
	return info, nil
}

// rewriteResultsURL points results_url at this gateway: Anthropic's is an
// absolute api.anthropic.com URL the SDK would fetch around the gateway (and
// with the vendor ID, which the gateway does not hand out). It runs after the
// batch id was wrapped. Without a configured public URL it is root-relative,
// which the Python and TypeScript SDKs resolve against the client's base URL.
func rewriteResultsURL(obj map[string]any, env nativeapi.TransformEnv) {
	if u, ok := obj["results_url"].(string); !ok || u == "" {
		return
	}
	id, _ := obj["id"].(string)
	if id == "" {
		obj["results_url"] = nil
		return
	}
	obj["results_url"] = strings.TrimSuffix(env.PublicBaseURL, "/") + "/v1/messages/batches/" + id + "/results"
}

// listEnvelope renders a page of batches the way Anthropic lists them;
// first_id and last_id are null on an empty page.
func listEnvelope(items [][]byte, firstID, lastID string, hasMore bool) ([]byte, error) {
	data := make([]json.RawMessage, len(items))
	for i, it := range items {
		data[i] = it
	}
	page := map[string]any{"data": data, "has_more": hasMore, "first_id": nil, "last_id": nil}
	if firstID != "" {
		page["first_id"] = firstID
	}
	if lastID != "" {
		page["last_id"] = lastID
	}
	return json.Marshal(page)
}

// NativeError renders an Anthropic-shaped error body.
func NativeError(status int, typ, msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": typ, "message": msg},
	})
	return b
}

// messagesFoldPaths are the objects of a Messages request the gateway decodes
// into Go structs, whose fields encoding/json matches case-insensitively:
// the request (the filter extractor, the stream check), each message, and
// tool_choice. Case variants of a key are refused there; everywhere else
// (tool schemas, tool_use inputs, metadata) only exact duplicates are.
var messagesFoldPaths = []string{"", "messages[]", "tool_choice"}

// checkMessagesKeys refuses a Messages request with duplicate keys the
// gateway and Anthropic could read differently (nativeapi.CheckJSONKeys).
func checkMessagesKeys(body []byte) error {
	return nativeapi.CheckJSONKeys(body, messagesFoldPaths...)
}

// errCountTokensBody is a count_tokens body that is not a well-formed
// Messages request.
var errCountTokensBody = errors.New("invalid count_tokens body")

// splitCountTokens admits a count_tokens body as one item: a Messages request
// with a model and no duplicate keys at any depth.
func splitCountTokens(body []byte) ([]nativeapi.Item, error) {
	model, err := messagesModel(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errCountTokensBody, err)
	}
	return []nativeapi.Item{{Format: "anthropic", Model: model, Body: body}}, nil
}

// joinSingle is the body of a single-item operation after the filters.
func joinSingle(_ []byte, items []nativeapi.Item) ([]byte, error) {
	if len(items) != 1 {
		return nil, fmt.Errorf("%d items for a single-body request", len(items))
	}
	return items[0].Body, nil
}

// batchItemSpan is one request of a batch create body: its custom_id, the
// model its params ask for, and where its params object sits in the body.
type batchItemSpan struct {
	customID, model string
	start, end      int
}

// errBatchBody is a create body that is not a well-formed batch.
var errBatchBody = errors.New("invalid message batch body")

// splitBatch turns a create body ({"requests":[{"custom_id","params"}...]})
// into one admission item per request; an item's Body is its params, a
// Messages request.
func splitBatch(body []byte) ([]nativeapi.Item, error) {
	spans, err := batchItemSpans(body)
	if err != nil {
		return nil, err
	}
	items := make([]nativeapi.Item, len(spans))
	for i, sp := range spans {
		items[i] = nativeapi.Item{Key: sp.customID, Format: "anthropic", Model: sp.model, Body: body[sp.start:sp.end]}
	}
	return items, nil
}

// joinBatch splices the params of the items a filter changed into the
// original body. Everything else, other items included, stays byte for byte.
func joinBatch(original []byte, items []nativeapi.Item) ([]byte, error) {
	spans, err := batchItemSpans(original)
	if err != nil {
		return nil, err
	}
	if len(spans) != len(items) {
		return nil, fmt.Errorf("%w: %d items for %d requests", errBatchBody, len(items), len(spans))
	}
	var out bytes.Buffer
	last := 0
	for i, sp := range spans {
		if bytes.Equal(items[i].Body, original[sp.start:sp.end]) {
			continue
		}
		out.Write(original[last:sp.start])
		out.Write(items[i].Body)
		last = sp.end
	}
	if last == 0 {
		return original, nil
	}
	out.Write(original[last:])
	return out.Bytes(), nil
}

// batchItemSpans parses a create body. Duplicate keys are refused wherever
// the gateway reads a value the vendor also reads (requests, custom_id,
// params, and everything inside params, at any depth): JSON parsers disagree
// on which duplicate wins, so a duplicate could show the gateway's checks or
// filters one model or prompt and the vendor another. Every error wraps
// errBatchBody once.
func batchItemSpans(body []byte) ([]batchItemSpan, error) {
	spans, err := parseBatch(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBatchBody, err)
	}
	return spans, nil
}

func parseBatch(body []byte) ([]batchItemSpan, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := expectDelim(dec, '{'); err != nil {
		return nil, err
	}
	var spans []batchItemSpan
	seenRequests := false
	seen := map[string]bool{}
	for dec.More() {
		key, err := objectKey(dec, seen)
		if err != nil {
			return nil, err
		}
		if key != "requests" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, err
			}
			continue
		}
		seenRequests = true
		if err := expectDelim(dec, '['); err != nil {
			return nil, err
		}
		for dec.More() {
			var el json.RawMessage
			if err := dec.Decode(&el); err != nil {
				return nil, err
			}
			end := int(dec.InputOffset())
			sp, err := batchItem(el, end-len(el))
			if err != nil {
				return nil, fmt.Errorf("request %d: %w", len(spans), err)
			}
			spans = append(spans, sp)
		}
		if err := expectDelim(dec, ']'); err != nil {
			return nil, err
		}
	}
	if err := expectDelim(dec, '}'); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	if !seenRequests || len(spans) == 0 {
		return nil, errors.New("requests must be a non-empty array")
	}
	return spans, nil
}

// batchItem reads one request object starting at offset off of the body.
func batchItem(el json.RawMessage, off int) (batchItemSpan, error) {
	dec := json.NewDecoder(bytes.NewReader(el))
	if err := expectDelim(dec, '{'); err != nil {
		return batchItemSpan{}, err
	}
	sp := batchItemSpan{start: -1}
	seen := map[string]bool{}
	for dec.More() {
		key, err := objectKey(dec, seen)
		if err != nil {
			return batchItemSpan{}, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return batchItemSpan{}, err
		}
		switch key {
		case "custom_id":
			if err := json.Unmarshal(v, &sp.customID); err != nil || sp.customID == "" {
				return batchItemSpan{}, errors.New("custom_id must be a non-empty string")
			}
		case "params":
			end := int(dec.InputOffset())
			sp.start, sp.end = off+end-len(v), off+end
			model, err := paramsModel(v)
			if err != nil {
				return batchItemSpan{}, err
			}
			sp.model = model
		}
	}
	if sp.customID == "" {
		return batchItemSpan{}, errors.New("custom_id is required")
	}
	if sp.start < 0 {
		return batchItemSpan{}, errors.New("params is required")
	}
	return sp, nil
}

// paramsModel returns the model of a params object, refusing duplicate keys
// at any depth (a message with two "content" keys as much as two "model"s).
func paramsModel(params json.RawMessage) (string, error) {
	model, err := messagesModel(params)
	if err != nil {
		return "", fmt.Errorf("params: %w", err)
	}
	return model, nil
}

// messagesModel returns the model of a Messages request object, refusing
// duplicate keys at any depth.
func messagesModel(body []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := expectDelim(dec, '{'); err != nil {
		return "", errors.New("must be an object")
	}
	if err := checkMessagesKeys(body); err != nil {
		return "", err
	}
	model := ""
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return "", err
		}
		if tok == "model" {
			if err := json.Unmarshal(v, &model); err != nil {
				return "", errors.New("model must be a string")
			}
		}
	}
	if model == "" {
		return "", errors.New("model is required")
	}
	return model, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != want {
		return fmt.Errorf("expected %q", want)
	}
	return nil
}

func objectKey(dec *json.Decoder, seen map[string]bool) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", errors.New("expected an object key")
	}
	// Go's encoding/json (the gateway's filter extractors) matches keys case
	// insensitively, under Unicode simple folding, with the last match
	// winning; the vendor matches them exactly. Keys equal under folding are
	// duplicates for that reason: {"messages":[x],"Messages":[y]} would show
	// the filters y and the vendor x.
	folded := foldKey(key)
	if seen[folded] {
		return "", fmt.Errorf("%w %q", nativeapi.ErrDuplicateKey, key)
	}
	seen[folded] = true
	return key, nil
}

// foldKey maps key to a canonical form under Unicode simple case folding, so
// foldKey(a) == foldKey(b) exactly when strings.EqualFold(a, b).
func foldKey(key string) string { return nativeapi.FoldKey(key) }

// BatchPriceMultiplier is the share of the list price Anthropic bills batch
// usage at.
const BatchPriceMultiplier = 0.5

// tierMultipliers prices the service tiers a batch result may report. A
// result without a tier is a batch result.
var tierMultipliers = map[string]float64{
	"":         BatchPriceMultiplier,
	"batch":    BatchPriceMultiplier,
	"standard": 1,
	"priority": 1,
}

// resultLine is one line of a batch's results file.
type resultLine struct {
	CustomID string `json:"custom_id"`
	Result   struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
	} `json:"result"`
}

// resultMessage is the part of a succeeded result's message the gateway
// bills.
type resultMessage struct {
	Model string `json:"model"`
	Usage struct {
		InputTokens              int    `json:"input_tokens"`
		OutputTokens             int    `json:"output_tokens"`
		CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
		ServiceTier              string `json:"service_tier"`
	} `json:"usage"`
}

// viewResultLine parses a results line. Only a succeeded result has a
// message and billed usage; errored, canceled and expired requests are not
// charged.
func viewResultLine(line []byte) (nativeapi.ResultView, error) {
	var l resultLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nativeapi.ResultView{}, err
	}
	if l.CustomID == "" || l.Result.Type == "" {
		return nativeapi.ResultView{}, errors.New("not a message batch result")
	}
	v := nativeapi.ResultView{Key: l.CustomID}
	if l.Result.Type != "succeeded" || len(l.Result.Message) == 0 {
		return v, nil
	}
	var m resultMessage
	if err := json.Unmarshal(l.Result.Message, &m); err != nil {
		return nativeapi.ResultView{}, err
	}
	mult, ok := tierMultipliers[m.Usage.ServiceTier]
	if !ok {
		mult = BatchPriceMultiplier
	}
	tier := m.Usage.ServiceTier
	if tier == "" {
		tier = "batch"
	}
	v.Model, v.Message = m.Model, l.Result.Message
	v.Usage = nativeapi.Usage{
		Model:                    m.Model,
		InputTokens:              m.Usage.InputTokens,
		OutputTokens:             m.Usage.OutputTokens,
		CacheCreationInputTokens: m.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     m.Usage.CacheReadInputTokens,
		Tier:                     tier,
		Multiplier:               mult,
		Billed:                   true,
	}
	return v, nil
}

// blockedResultLine is the line that replaces a result a response filter
// blocked: an errored result for the same custom_id.
func blockedResultLine(line []byte, msg string) ([]byte, error) {
	var l resultLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"custom_id": l.CustomID,
		"result": map[string]any{
			"type": "errored",
			"error": map[string]any{
				"type":  "error",
				"error": map[string]string{"type": "invalid_request_error", "message": msg},
			},
		},
	})
}
