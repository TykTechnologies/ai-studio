package openaiVendor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

// OpenAI Batches and Files on the native routes: a batch input file is
// uploaded (POST /v1/files, purpose=batch) and every request line in it is
// admitted like a synchronous call, a batch is created from the file's ID,
// and its usage is read from its output file once it ends and billed at the
// batch price.

const (
	// ResourceFile is the native resource type of an uploaded or batch
	// output file.
	ResourceFile = "openai.file"
	// ResourceBatch is the native resource type of a batch.
	ResourceBatch = "openai.batch"
)

// DefaultModerationModel is the model OpenAI runs a moderation on when the
// request names none ("model" is optional on /v1/moderations). The gateway
// checks LLM allow-lists against it.
const DefaultModerationModel = "omni-moderation-latest"

// IsModerationsPath reports whether path is the moderations endpoint
// (/v1/moderations, or /moderations under an endpoint prefix).
func IsModerationsPath(path string) bool {
	return strings.HasSuffix(strings.TrimSuffix(strings.ToLower(path), "/"), "/moderations")
}

// BatchPriceMultiplier is the share of the list price OpenAI bills batch
// usage at.
const BatchPriceMultiplier = 0.5

// batchOutputRetention is how long the gateway expects a batch's output file
// to stay downloadable after the batch was created.
const batchOutputRetention = 30 * 24 * time.Hour

var (
	fileIDCharset  = regexp.MustCompile(`^file-[A-Za-z0-9_-]{1,200}$`)
	batchIDCharset = regexp.MustCompile(`^batch_[A-Za-z0-9_-]{1,200}$`)
)

// batchEndpoints are the request URLs a batch input line may name, and the
// message format the request filters read its body as.
var batchEndpoints = map[string]string{
	"/v1/chat/completions": "openai",
	"/v1/completions":      InputFilterFormat,
	"/v1/embeddings":       InputFilterFormat,
	"/v1/responses":        ResponsesFilterFormat,
	"/v1/moderations":      InputFilterFormat,
}

// batchFileIDs are the file IDs inside a batch object; the output and error
// files are children of the batch.
var batchFileIDs = []nativeapi.BodyRef{
	{Path: "id", Resource: ResourceBatch},
	{Path: "input_file_id", Resource: ResourceFile},
	{Path: "output_file_id", Resource: ResourceFile, Child: true},
	{Path: "error_file_id", Resource: ResourceFile, Child: true},
}

var fileIDs = []nativeapi.BodyRef{{Path: "id", Resource: ResourceFile}}

var batchResources = []nativeapi.ResourceType{
	{Name: ResourceFile, IDCharset: fileIDCharset},
	{
		Name:      ResourceBatch,
		IDCharset: batchIDCharset,
		Async: &nativeapi.AsyncSpec{
			Poll: func(vendorID string) nativeapi.UpstreamReq {
				return nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/batches/" + vendorID}
			},
			State: batchState,
			Results: func(_ string, obj []byte) (nativeapi.UpstreamReq, bool) {
				var b batchObject
				if json.Unmarshal(obj, &b) != nil || b.OutputFileID == nil || *b.OutputFileID == "" ||
					!fileIDCharset.MatchString(*b.OutputFileID) {
					return nativeapi.UpstreamReq{}, false
				}
				return nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/files/" + *b.OutputFileID + "/content"}, true
			},
			LineUsage: func(line []byte) (nativeapi.Usage, bool, error) {
				v, err := viewBatchLine(line)
				if err != nil || !v.Usage.Billed {
					return nativeapi.Usage{}, false, err
				}
				return v.Usage, true, nil
			},
			Cancel: func(vendorID string) nativeapi.UpstreamReq {
				return nativeapi.UpstreamReq{Method: http.MethodPost, Path: "/v1/batches/" + vendorID + "/cancel"}
			},
		},
	},
}

var batchOperations = []nativeapi.Operation{
	{
		Name:        "openai.files.create",
		Method:      http.MethodPost,
		Path:        "/v1/files",
		Ledger:      nativeapi.LedgerCreate,
		Admit:       &nativeapi.AdmitSpec{Multipart: true, Split: splitBatchFile, Join: joinBatchFile},
		ResponseIDs: fileIDs,
		Upstream:    nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:  NativeError,
	},
	{
		// Served from this gateway's ledger: the vendor would list every
		// App's files in the project.
		Name:        "openai.files.list",
		Method:      http.MethodGet,
		Path:        "/v1/files",
		Ledger:      nativeapi.LedgerList,
		ResponseIDs: []nativeapi.BodyRef{{Path: "data[].id", Resource: ResourceFile}},
		List: &nativeapi.ListSpec{
			Envelope:   listEnvelope,
			LimitParam: "limit", AfterParam: "after",
			DefaultLimit: 1000, MaxLimit: 1000,
		},
		ErrorShape: NativeError,
	},
	{
		Name:         "openai.files.retrieve",
		Method:       http.MethodGet,
		Path:         "/v1/files/{id}",
		PathResource: ResourceFile,
		Ledger:       nativeapi.LedgerRead,
		ResponseIDs:  fileIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.files.delete",
		Method:       http.MethodDelete,
		Path:         "/v1/files/{id}",
		PathResource: ResourceFile,
		Ledger:       nativeapi.LedgerDelete,
		ResponseIDs:  fileIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.files.content",
		Method:       http.MethodGet,
		Path:         "/v1/files/{id}/content",
		PathResource: ResourceFile,
		Ledger:       nativeapi.LedgerRead,
		Results:      &nativeapi.ResultsSpec{View: viewBatchLine, Blocked: blockedBatchLine},
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow, LongRunning: true},
		ErrorShape:   NativeError,
	},
	{
		Name:        "openai.batches.create",
		Method:      http.MethodPost,
		Path:        "/v1/batches",
		Ledger:      nativeapi.LedgerCreate,
		Budget:      true,
		RequestRefs: []nativeapi.BodyRef{{Path: "input_file_id", Resource: ResourceFile}},
		ResponseIDs: batchFileIDs,
		Upstream:    nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:  NativeError,
	},
	{
		Name:        "openai.batches.list",
		Method:      http.MethodGet,
		Path:        "/v1/batches",
		Ledger:      nativeapi.LedgerList,
		ResponseIDs: []nativeapi.BodyRef{{Path: "data[].id", Resource: ResourceBatch}},
		List: &nativeapi.ListSpec{
			Envelope:   listEnvelope,
			LimitParam: "limit", AfterParam: "after",
			DefaultLimit: 20, MaxLimit: 100,
		},
		ErrorShape: NativeError,
	},
	{
		Name:         "openai.batches.retrieve",
		Method:       http.MethodGet,
		Path:         "/v1/batches/{id}",
		PathResource: ResourceBatch,
		Ledger:       nativeapi.LedgerRead,
		ResponseIDs:  batchFileIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
	{
		Name:         "openai.batches.cancel",
		Method:       http.MethodPost,
		Path:         "/v1/batches/{id}/cancel",
		PathResource: ResourceBatch,
		Ledger:       nativeapi.LedgerMutate,
		ResponseIDs:  batchFileIDs,
		Upstream:     nativeapi.UpstreamSpec{HeaderAllow: nativeHeaderAllow},
		ErrorShape:   NativeError,
	},
}

// listEnvelope renders a page the way OpenAI lists files and batches.
func listEnvelope(items [][]byte, firstID, lastID string, hasMore bool) ([]byte, error) {
	data := make([]json.RawMessage, len(items))
	for i, it := range items {
		data[i] = it
	}
	page := map[string]any{"object": "list", "data": data, "has_more": hasMore, "first_id": nil, "last_id": nil}
	if firstID != "" {
		page["first_id"] = firstID
	}
	if lastID != "" {
		page["last_id"] = lastID
	}
	return json.Marshal(page)
}

// batchObject is the part of a batch object the gateway reads.
type batchObject struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	OutputFileID *string `json:"output_file_id"`
	CreatedAt    int64   `json:"created_at"`
	CompletedAt  *int64  `json:"completed_at"`
	FailedAt     *int64  `json:"failed_at"`
	ExpiredAt    *int64  `json:"expired_at"`
	CancelledAt  *int64  `json:"cancelled_at"`
}

// batchState maps a batch's status onto the coarse lifecycle the reconciler
// drives.
func batchState(obj []byte) (nativeapi.StateInfo, error) {
	var b batchObject
	if err := json.Unmarshal(obj, &b); err != nil {
		return nativeapi.StateInfo{}, err
	}
	if b.ID == "" || b.Status == "" {
		return nativeapi.StateInfo{}, errors.New("not a batch object")
	}
	var info nativeapi.StateInfo
	switch b.Status {
	case "completed", "failed", "expired", "cancelled":
		info.State = nativeapi.StateEnded
		for _, at := range []*int64{b.CompletedAt, b.FailedAt, b.ExpiredAt, b.CancelledAt} {
			if at != nil && *at > 0 {
				t := time.Unix(*at, 0).UTC()
				info.EndedAt = &t
				break
			}
		}
	case "cancelling":
		info.State = nativeapi.StateCanceling
	default: // validating, in_progress, finalizing
		info.State = nativeapi.StateInProgress
	}
	if b.CreatedAt > 0 {
		exp := time.Unix(b.CreatedAt, 0).UTC().Add(batchOutputRetention)
		info.ResultsExpireAt = &exp
	}
	return info, nil
}

// batchLine is one line of a batch output or error file.
type batchLine struct {
	ID       string `json:"id"`
	CustomID string `json:"custom_id"`
	Response *struct {
		StatusCode int             `json:"status_code"`
		RequestID  string          `json:"request_id"`
		Body       json.RawMessage `json:"body"`
	} `json:"response"`
}

// batchLineBody is the part of a result body the gateway bills: a chat
// completion or embedding (prompt_tokens) or a response (input_tokens).
type batchLineBody struct {
	Model string `json:"model"`
	Usage *struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
	} `json:"usage"`
}

// viewBatchLine parses an output or error file line. Only a 200 result has a
// message and billed usage.
func viewBatchLine(line []byte) (nativeapi.ResultView, error) {
	var l batchLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nativeapi.ResultView{}, err
	}
	if l.CustomID == "" {
		return nativeapi.ResultView{}, errors.New("not a batch result")
	}
	v := nativeapi.ResultView{Key: l.CustomID}
	if l.Response == nil || l.Response.StatusCode != http.StatusOK || len(l.Response.Body) == 0 {
		return v, nil
	}
	var b batchLineBody
	if err := json.Unmarshal(l.Response.Body, &b); err != nil {
		return nativeapi.ResultView{}, err
	}
	v.Model, v.Message = b.Model, l.Response.Body
	if b.Usage == nil {
		return v, nil
	}
	in, out, cached := b.Usage.PromptTokens, b.Usage.CompletionTokens, b.Usage.PromptTokensDetails.CachedTokens
	if in == 0 && out == 0 {
		in, out, cached = b.Usage.InputTokens, b.Usage.OutputTokens, b.Usage.InputTokensDetails.CachedTokens
	}
	v.Usage = nativeapi.Usage{
		Model:                b.Model,
		InputTokens:          in,
		OutputTokens:         out,
		CacheReadInputTokens: cached,
		Tier:                 "batch",
		Multiplier:           BatchPriceMultiplier,
		Billed:               true,
	}
	return v, nil
}

// blockedBatchLine replaces a result a response filter blocked with a failed
// result for the same custom_id.
func blockedBatchLine(line []byte, msg string) ([]byte, error) {
	var l batchLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil, err
	}
	requestID := ""
	if l.Response != nil {
		requestID = l.Response.RequestID
	}
	return json.Marshal(map[string]any{
		"id":        l.ID,
		"custom_id": l.CustomID,
		"response": map[string]any{
			"status_code": http.StatusBadRequest,
			"request_id":  requestID,
			"body":        json.RawMessage(NativeError(http.StatusBadRequest, "invalid_request_error", msg)),
		},
		"error": nil,
	})
}

// --- batch input file upload ------------------------------------------------

// errBatchFile is an upload the gateway does not accept.
var errBatchFile = errors.New("invalid batch file upload")

// batchFilePart is one part of a file upload.
type batchFilePart struct {
	header  textproto.MIMEHeader
	content []byte
	isFile  bool
}

// batchLineSpan is one request line of an uploaded batch file: its
// custom_id, model, filter format, and where its body sits in the file.
type batchLineSpan struct {
	customID, model, format string
	start, end              int
}

// uploadFields are the form fields a batch file upload may carry.
var uploadFields = map[string]bool{
	"file": true, "purpose": true, "expires_after[anchor]": true, "expires_after[seconds]": true,
}

// uploadDisposition is the only Content-Disposition a part may carry: the
// plain form the SDKs send (form-data; name="..."[; filename="..."]). No
// quoted-pair escapes, no RFC 2231 name*/filename* parameters, no repeated or
// other parameters: parsers disagree on all of them (Go's keeps a backslash
// and prefers name*, Python's unescapes it and may prefer name), and a part
// the gateway names one way must not be another part to the vendor.
var uploadDisposition = regexp.MustCompile(`^form-data; name="([A-Za-z0-9_.\[\]-]{1,64})"(?:; filename="([^"\\\r\n]{0,255})")?$`)

// parseBatchUpload parses a multipart upload whose body opens with its
// boundary (the proxy checks it is the Content-Type's). It fails closed on
// anything a vendor's parser might read differently: parts must carry the
// plain Content-Disposition above and only a Content-Type besides; names are
// allow-listed and unique; only the file part has a filename; no part may
// carry a transfer encoding; nothing may follow the closing boundary.
// Exactly one file and one purpose are required, and the purpose must be
// batch.
func parseBatchUpload(body []byte) (boundary string, parts []batchFilePart, file int, err error) {
	first, _, ok := bytes.Cut(body, []byte("\n"))
	first = bytes.TrimSuffix(first, []byte("\r"))
	if !ok || !bytes.HasPrefix(first, []byte("--")) || len(first) <= 2 {
		return "", nil, 0, errors.New("the body must be multipart/form-data opening with its boundary")
	}
	boundary = string(first[2:])
	// The closing delimiter ends the body: an epilogue could hold parts a
	// lenient parser still reads.
	closing := []byte("\n--" + boundary + "--")
	i := bytes.Index(body, closing)
	if i < 0 {
		return "", nil, 0, errors.New("the body has no closing boundary")
	}
	if rest := body[i+len(closing):]; len(rest) > 0 && !bytes.Equal(rest, []byte("\r\n")) && !bytes.Equal(rest, []byte("\n")) {
		return "", nil, 0, errors.New("nothing may follow the closing boundary")
	}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	file = -1
	purpose, seen := "", map[string]bool{}
	for {
		p, err := mr.NextRawPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, 0, err
		}
		for k, vs := range p.Header {
			if (k != "Content-Disposition" && k != "Content-Type") || len(vs) != 1 {
				return "", nil, 0, fmt.Errorf("unsupported part header %q", k)
			}
		}
		m := uploadDisposition.FindStringSubmatch(p.Header.Get("Content-Disposition"))
		if m == nil {
			return "", nil, 0, errors.New(`every part needs a plain Content-Disposition: form-data; name="..."`)
		}
		name, hasFilename := m[1], strings.Contains(p.Header.Get("Content-Disposition"), "; filename=")
		if !uploadFields[name] {
			return "", nil, 0, fmt.Errorf("unsupported form field %q", name)
		}
		if seen[name] {
			return "", nil, 0, fmt.Errorf("duplicate form field %q", name)
		}
		seen[name] = true
		if hasFilename != (name == "file") {
			return "", nil, 0, errors.New("only the file part may (and must) have a filename")
		}
		content, err := io.ReadAll(p)
		if err != nil {
			return "", nil, 0, err
		}
		part := batchFilePart{header: p.Header, content: content}
		switch name {
		case "file":
			part.isFile, file = true, len(parts)
		case "purpose":
			purpose = string(content)
		}
		parts = append(parts, part)
	}
	if file < 0 {
		return "", nil, 0, errors.New("file is required")
	}
	if purpose != "batch" {
		return "", nil, 0, errors.New(`only files with purpose "batch" can be uploaded through the gateway`)
	}
	return boundary, parts, file, nil
}

// batchLineFoldPaths are the objects of a batch input line the gateway decodes
// into Go structs (encoding/json matches their fields case-insensitively):
// the line, its body (model, and the chat request the filters read), each
// chat message and tool. Case variants of a key are refused there; elsewhere
// (tool parameter schemas, metadata) only exact duplicates are.
var batchLineFoldPaths = []string{
	"", "body", "body.messages[]", "body.tools[]", "body.tools[].function",
	"body.response_format", "body.audio", "body.stream_options",
}

// batchFileLines parses a batch input file: one JSON request per non-blank
// line, with a custom_id, method POST, a supported url and a body naming a
// model. Duplicate keys are refused at every depth, so the gateway's checks
// and the vendor read the same line.
func batchFileLines(content []byte) ([]batchLineSpan, error) {
	var spans []batchLineSpan
	off := 0
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 64<<10), nativeapi.DefaultMaxLineSize)
	sc.Split(scanRawLines)
	for sc.Scan() {
		raw := sc.Bytes()
		lineStart := off
		off += len(raw)
		line := bytes.TrimRight(raw, "\r\n")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		n := len(spans)
		if err := nativeapi.CheckJSONKeys(line, batchLineFoldPaths...); err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		sp, err := batchFileLine(line, lineStart)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		spans = append(spans, sp)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, errors.New("the file holds no requests")
	}
	return spans, nil
}

// scanRawLines is bufio.ScanLines that keeps the line terminator, so offsets
// add up to the file's bytes.
func scanRawLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// batchFileLine reads one request line starting at offset off of the file.
func batchFileLine(line []byte, off int) (batchLineSpan, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return batchLineSpan{}, errors.New("a request must be a JSON object")
	}
	sp := batchLineSpan{start: -1}
	method, url := "", ""
	modelMissing := false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return batchLineSpan{}, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return batchLineSpan{}, err
		}
		switch tok {
		case "custom_id":
			if json.Unmarshal(v, &sp.customID) != nil || sp.customID == "" {
				return batchLineSpan{}, errors.New("custom_id must be a non-empty string")
			}
		case "method":
			_ = json.Unmarshal(v, &method)
		case "url":
			var u string
			_ = json.Unmarshal(v, &u)
			url = u
			format, ok := batchEndpoints[u]
			if !ok {
				return batchLineSpan{}, fmt.Errorf("url %q is not supported in a batch", u)
			}
			sp.format = format
		case "body":
			end := int(dec.InputOffset())
			sp.start, sp.end = off+end-len(v), off+end
			var b struct {
				Model *string `json:"model"`
			}
			if json.Unmarshal(v, &b) != nil {
				return batchLineSpan{}, errors.New("body must be an object")
			}
			if b.Model == nil || *b.Model == "" {
				modelMissing = true
			} else {
				sp.model = *b.Model
			}
		}
	}
	switch {
	case sp.customID == "":
		return batchLineSpan{}, errors.New("custom_id is required")
	case method != http.MethodPost:
		return batchLineSpan{}, errors.New(`method must be "POST"`)
	case sp.format == "":
		return batchLineSpan{}, errors.New("url is required")
	case sp.start < 0:
		return batchLineSpan{}, errors.New("body is required")
	case modelMissing && !IsModerationsPath(url):
		return batchLineSpan{}, errors.New("body.model is required")
	case modelMissing:
		// A moderation without a model runs on OpenAI's default; the
		// allow-list is checked against that.
		sp.model = DefaultModerationModel
	}
	return sp, nil
}

// splitBatchFile turns a batch file upload into one admission item per
// request line; an item's Body is the line's request body.
func splitBatchFile(body []byte) ([]nativeapi.Item, error) {
	_, parts, file, err := parseBatchUpload(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBatchFile, err)
	}
	content := parts[file].content
	spans, err := batchFileLines(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBatchFile, err)
	}
	items := make([]nativeapi.Item, len(spans))
	for i, sp := range spans {
		items[i] = nativeapi.Item{Key: sp.customID, Format: sp.format, Model: sp.model, Body: content[sp.start:sp.end]}
	}
	return items, nil
}

// joinBatchFile splices the bodies a filter changed into the file and
// re-encodes the upload with the same boundary, so the Content-Type the
// client sent still describes it. Unchanged lines stay byte for byte.
func joinBatchFile(original []byte, items []nativeapi.Item) ([]byte, error) {
	boundary, parts, file, err := parseBatchUpload(original)
	if err != nil {
		return nil, err
	}
	content := parts[file].content
	spans, err := batchFileLines(content)
	if err != nil {
		return nil, err
	}
	if len(spans) != len(items) {
		return nil, fmt.Errorf("%w: %d items for %d requests", errBatchFile, len(items), len(spans))
	}
	var joined bytes.Buffer
	last := 0
	for i, sp := range spans {
		if bytes.Equal(items[i].Body, content[sp.start:sp.end]) {
			continue
		}
		// A line stays one line.
		if bytes.ContainsAny(items[i].Body, "\r\n") {
			var compact bytes.Buffer
			if err := json.Compact(&compact, items[i].Body); err != nil {
				return nil, err
			}
			items[i].Body = compact.Bytes()
		}
		joined.Write(content[last:sp.start])
		joined.Write(items[i].Body)
		last = sp.end
	}
	if last == 0 {
		return original, nil
	}
	joined.Write(content[last:])
	parts[file].content = joined.Bytes()

	var out bytes.Buffer
	mw := multipart.NewWriter(&out)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, err
	}
	for _, p := range parts {
		w, err := mw.CreatePart(p.header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(p.content); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
