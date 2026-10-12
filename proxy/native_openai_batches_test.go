package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

// --- fake OpenAI Files and Batches -------------------------------------------

type fakeOpenAIFile struct {
	obj     map[string]any
	content []byte
}

// serveBatches answers the Files and Batches API; false when the path is
// not one of theirs.
func (f *fakeResponsesAPI) serveBatches(w http.ResponseWriter, r *http.Request, body []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	writeJSON := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write(b)
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
		req, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(openaiVendor.NativeError(400, "invalid_request_error", err.Error()))
			return true
		}
		fh := req.MultipartForm.File["file"][0]
		fr, _ := fh.Open()
		content, _ := io.ReadAll(fr)
		f.n++
		id := fmt.Sprintf("file-in%d", f.n)
		f.files[id] = &fakeOpenAIFile{content: content, obj: map[string]any{
			"id": id, "object": "file", "bytes": len(content), "created_at": time.Now().Unix(),
			"filename": fh.Filename, "purpose": req.FormValue("purpose"),
		}}
		writeJSON(f.files[id].obj)
	case strings.HasPrefix(r.URL.Path, "/v1/files/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/files/"), "/")
		file, ok := f.files[id]
		switch {
		case !ok:
			f.notFound(w)
		case r.Method == http.MethodGet && action == "content":
			if f.failContent {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write(openaiVendor.NativeError(500, "api_error", "try again"))
				return true
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(file.content)
		case r.Method == http.MethodGet && action == "":
			writeJSON(file.obj)
		case r.Method == http.MethodDelete && action == "":
			delete(f.files, id)
			writeJSON(map[string]any{"id": id, "object": "file", "deleted": true})
		default:
			f.notFound(w)
		}
	case r.Method == http.MethodPost && r.URL.Path == "/v1/batches":
		var req struct {
			InputFileID string `json:"input_file_id"`
			Endpoint    string `json:"endpoint"`
		}
		_ = json.Unmarshal(body, &req)
		if f.files[req.InputFileID] == nil {
			f.notFound(w)
			return true
		}
		f.n++
		id := fmt.Sprintf("batch_%d", f.n)
		f.batches[id] = map[string]any{
			"id": id, "object": "batch", "endpoint": req.Endpoint, "input_file_id": req.InputFileID,
			"completion_window": "24h", "status": "validating", "output_file_id": nil, "error_file_id": nil,
			"created_at": time.Now().Unix(), "completed_at": nil,
			"request_counts": map[string]any{"total": 2, "completed": 0, "failed": 0},
		}
		writeJSON(f.batches[id])
	case strings.HasPrefix(r.URL.Path, "/v1/batches/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/v1/batches/"), "/")
		b, ok := f.batches[id]
		switch {
		case !ok:
			f.notFound(w)
		case r.Method == http.MethodGet && action == "":
			writeJSON(b)
		case r.Method == http.MethodPost && action == "cancel":
			b["status"] = "cancelling"
			writeJSON(b)
		default:
			f.notFound(w)
		}
	default:
		return false
	}
	return true
}

// completeBatch ends a batch with an output file holding lines.
func (f *fakeResponsesAPI) completeBatch(id string, lines ...string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	out := fmt.Sprintf("file-out%d", f.n)
	f.files[out] = &fakeOpenAIFile{content: []byte(strings.Join(lines, "\n") + "\n"), obj: map[string]any{
		"id": out, "object": "file", "bytes": 10, "created_at": time.Now().Unix(), "filename": "batch_output.jsonl", "purpose": "batch_output",
	}}
	b := f.batches[id]
	b["status"], b["output_file_id"], b["completed_at"] = "completed", out, time.Now().Unix()
	return out
}

func outputLine(customID string, prompt, completion, cached int) string {
	return fmt.Sprintf(`{"id":"batch_req_%s","custom_id":"%s","response":{"status_code":200,"request_id":"req_%s","body":`+
		`{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o-2026","choices":[{"index":0,"message":{"role":"assistant","content":"ok %s"}}],`+
		`"usage":{"prompt_tokens":%d,"completion_tokens":%d,"prompt_tokens_details":{"cached_tokens":%d}}}},"error":null}`,
		customID, customID, customID, customID, prompt, completion, cached)
}

// --- helpers -------------------------------------------------------------------

func multipartUpload(t *testing.T, purpose, content string) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	require.NoError(t, mw.WriteField("purpose", purpose))
	fw, err := mw.CreateFormFile("file", "batch.jsonl")
	require.NoError(t, err)
	_, _ = io.WriteString(fw, content)
	require.NoError(t, mw.Close())
	return b.Bytes(), mw.FormDataContentType()
}

// doRaw sends body with contentType as the App holding key.
func (h *responsesHarness) doRaw(key, method, path, contentType string, body []byte) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(method, "http://127.0.0.1:"+strconv.Itoa(h.port)+path, bytes.NewReader(body))
	require.NoError(h.t, err)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)
	return resp, out
}

func (h *responsesHarness) uploadFile(key string, lines ...string) (*http.Response, []byte) {
	h.t.Helper()
	body, ct := multipartUpload(h.t, "batch", strings.Join(lines, "\n"))
	return h.doRaw(key, http.MethodPost, "/llm/call/primary/v1/files", ct, body)
}

func chatLine(customID, model, content string) string {
	return `{"custom_id":"` + customID + `","method":"POST","url":"/v1/chat/completions","body":{"model":"` + model +
		`","messages":[{"role":"user","content":"` + content + `"}]}}`
}

func vendorPart(id string) string {
	v, _, _ := strings.Cut(id, ".")
	return v
}

func newBatchesHarness(t *testing.T, opts ...nativeHarnessOpts) *responsesHarness {
	h := newResponsesHarness(t, opts...)
	h.primary.AllowedModels = []string{"^gpt-4o"}
	require.NoError(t, h.db.Save(h.primary).Error)
	require.NoError(t, h.proxy.loadResources())
	return h
}

// --- end to end ----------------------------------------------------------------

func TestOpenAIBatches_FullFlowAccountedOnceAtBatchPrice(t *testing.T) {
	h := newBatchesHarness(t)
	ctx := context.Background()
	store := h.service.NativeResourceStore()

	// Upload: every line is admitted, the file ID handed out is signed.
	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"), chatLine("r2", "gpt-4o-mini", "there"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file := decodeObj(t, out)["id"].(string)
	assert.Contains(t, file, ".t1k")
	assert.Equal(t, "file-in1", vendorPart(file))
	uploads := h.api.callsTo(http.MethodPost, "/v1/files")
	require.Len(t, uploads, 1)
	assert.Contains(t, string(uploads[0].Body), chatLine("r2", "gpt-4o-mini", "there"), "an unfiltered upload is forwarded as sent")

	// The App's own file list shows it; another App's does not.
	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/files", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	list := decodeObj(t, out)
	assert.Equal(t, "list", list["object"])
	require.Len(t, list["data"], 1)
	assert.Equal(t, file, list["data"].([]any)[0].(map[string]any)["id"])
	_, out = h.do(h.keyD, http.MethodGet, "/llm/call/primary/v1/files", "")
	assert.Empty(t, decodeObj(t, out)["data"])

	// Another App cannot build a batch on the file, nor can a raw ID.
	for _, tc := range []struct{ key, file string }{{h.keyD, file}, {h.keyC, "file-in1"}} {
		resp, out = h.do(tc.key, http.MethodPost, "/llm/call/primary/v1/batches",
			`{"input_file_id":"`+tc.file+`","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s", out)
	}
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/batches"))

	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches",
		`{"input_file_id":"`+file+`","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	created := decodeObj(t, out)
	batch := created["id"].(string)
	assert.Equal(t, file, created["input_file_id"])
	assert.Nil(t, created["output_file_id"])
	creates := h.api.callsTo(http.MethodPost, "/v1/batches")
	require.Len(t, creates, 1)
	assert.Equal(t, "file-in1", decodeObj(t, creates[0].Body)["input_file_id"])
	row, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceBatch, vendorPart(batch))
	require.NoError(t, err)
	assert.Equal(t, models.NativeAccountingPending, row.AccountingState)

	// Another App cannot see it.
	resp, _ = h.do(h.keyD, http.MethodGet, "/llm/call/primary/v1/batches/"+batch, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	_, out = h.do(h.keyD, http.MethodGet, "/llm/call/primary/v1/batches", "")
	assert.Empty(t, decodeObj(t, out)["data"])
	_, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/batches", "")
	require.Len(t, decodeObj(t, out)["data"], 1)

	// It ends; the retrieve wraps the output file and records it as the
	// batch's child.
	outFile := h.api.completeBatch(vendorPart(batch), outputLine("r1", 100, 20, 40), outputLine("r2", 10, 5, 0))
	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/batches/"+batch, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	wrappedOut := decodeObj(t, out)["output_file_id"].(string)
	assert.Equal(t, outFile, vendorPart(wrappedOut))
	child, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceFile, outFile)
	require.NoError(t, err)
	require.NotNil(t, child.ParentID)
	assert.Equal(t, row.ID, *child.ParentID)
	assert.Contains(t, child.Snapshot, `"purpose":"batch_output"`)
	_, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/files", "")
	assert.Len(t, decodeObj(t, out)["data"], 2, "input and output files")

	// Download: streamed through the gateway; another App gets a 404.
	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/files/"+wrappedOut+"/content", "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.Equal(t, 2, strings.Count(string(out), `"custom_id"`))
	resp, _ = h.do(h.keyD, http.MethodGet, "/llm/call/primary/v1/files/"+wrappedOut+"/content", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// The reconciler accounts it once, at half the list price.
	h.proxy.reconcileNative(ctx, store, time.Now())
	h.proxy.reconcileNative(ctx, store, time.Now().Add(time.Hour))
	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 1, "one record per model and price")
	assert.Equal(t, "gpt-4o-2026", recs[0].Name)
	assert.Equal(t, 110-40, recs[0].PromptTokens)
	assert.Equal(t, 40, recs[0].CacheReadPromptTokens)
	assert.Equal(t, 25, recs[0].ResponseTokens)
	assert.InDelta(t, 0.5*models.ChatRecordCost(string(models.OPENAI), 2, 1, 0, 0.5, 70, 25, 0, 40), recs[0].Cost, 1e-9)

	// Settled: the output file may go, and the ledger row with it.
	resp, out = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/files/"+wrappedOut, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	_, err = store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceFile, outFile)
	assert.ErrorIs(t, err, services.ErrNativeResourceNotFound)
	assert.Len(t, h.chatRecords(h.appC), 1)
}

func TestOpenAIBatches_UploadAdmission(t *testing.T) {
	h := newBatchesHarness(t)

	resp, out := h.uploadFile(h.keyC, chatLine("ok", "gpt-4o", "hi"), chatLine("bad-one", "o1-pro", "hi"))
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "%s", out)
	_, msg := oaiErr(t, out)
	assert.Contains(t, msg, `custom_id "bad-one"`)

	for name, lines := range map[string][]string{
		"duplicate model": {`{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","model":"o1-pro"}}`},
		"nested duplicate": {`{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o",` +
			`"messages":[{"role":"user","content":"a","Content":"b"}]}}`},
		"unsupported url": {`{"custom_id":"a","method":"POST","url":"/v1/files","body":{"model":"gpt-4o"}}`},
	} {
		resp, out = h.uploadFile(h.keyC, lines...)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, out)
	}

	body, ct := multipartUpload(t, "fine-tune", chatLine("a", "gpt-4o", "hi"))
	resp, out = h.doRaw(h.keyC, http.MethodPost, "/llm/call/primary/v1/files", ct, body)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", out)
	_, msg = oaiErr(t, out)
	assert.Contains(t, msg, `purpose "batch"`)

	// The body must open with the boundary the Content-Type names: a
	// preamble or another boundary would let the vendor read other parts.
	body, ct = multipartUpload(t, "batch", chatLine("a", "gpt-4o", "hi"))
	for name, c := range map[string]struct {
		ct   string
		body []byte
	}{
		"preamble":       {ct, append([]byte("--x\r\n\r\nignored by the vendor\r\n"), body...)},
		"other boundary": {"multipart/form-data; boundary=another", body},
		"not multipart":  {"application/json", body},
	} {
		resp, out = h.doRaw(h.keyC, http.MethodPost, "/llm/call/primary/v1/files", c.ct, c.body)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, out)
	}
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/files"))
}

func TestOpenAIBatches_OutputFileDeleteWaitsForAccounting(t *testing.T) {
	h := newBatchesHarness(t)
	ctx := context.Background()
	_, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hi"))
	file := decodeObj(t, out)["id"].(string)
	_, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches",
		`{"input_file_id":"`+file+`","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
	batch := decodeObj(t, out)["id"].(string)
	h.api.completeBatch(vendorPart(batch), outputLine("r1", 10, 5, 0))
	_, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/batches/"+batch, "")
	outFile := decodeObj(t, out)["output_file_id"].(string)

	// The results cannot be read yet: the delete is refused, the file kept.
	h.api.mu.Lock()
	h.api.failContent = true
	h.api.mu.Unlock()
	resp, out := h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/files/"+outFile, "")
	require.Equal(t, http.StatusConflict, resp.StatusCode, "%s", out)
	_, msg := oaiErr(t, out)
	assert.Contains(t, msg, "this file cannot be deleted until the batch it belongs to has ended")
	assert.Empty(t, h.api.callsTo(http.MethodDelete, "/v1/files/"+vendorPart(outFile)))
	assert.Empty(t, h.chatRecords(h.appC))

	// Once they can, the delete accounts the batch first.
	h.api.mu.Lock()
	h.api.failContent = false
	h.api.mu.Unlock()
	resp, out = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/files/"+outFile, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	require.Len(t, h.chatRecords(h.appC), 1)
	h.proxy.reconcileNative(ctx, h.service.NativeResourceStore(), time.Now().Add(time.Hour))
	assert.Len(t, h.chatRecords(h.appC), 1, "accounted exactly once")
}

func TestOpenAIBatches_InputFileOwnedOnItsNode(t *testing.T) {
	h := newBatchesHarness(t)
	_, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hi"))
	file := decodeObj(t, out)["id"].(string)

	resp, _ := h.do(h.keyD, http.MethodDelete, "/llm/call/primary/v1/files/"+file, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp, _ = h.do(h.keyD, http.MethodGet, "/llm/call/primary/v1/files/"+file, "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/files/"+file, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.Equal(t, file, decodeObj(t, out)["id"])
	resp, out = h.do(h.keyC, http.MethodDelete, "/llm/call/primary/v1/files/"+file, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	assert.Equal(t, true, decodeObj(t, out)["deleted"])
}

func TestOpenAIBatches_InProgressCap(t *testing.T) {
	h := newBatchesHarness(t, nativeHarnessOpts{config: &Config{NativeMaxInProgressPerApp: 1}})
	_, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hi"))
	file := decodeObj(t, out)["id"].(string)
	body := `{"input_file_id":"` + file + `","endpoint":"/v1/chat/completions","completion_window":"24h"}`
	resp, out := h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches", body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches", body)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "%s", out)
}

// A part the gateway and a vendor could name differently is refused: the
// unchecked part never reaches the vendor as its batch file.
func TestOpenAIBatches_AmbiguousPartsRefused(t *testing.T) {
	h := newBatchesHarness(t)
	const b = "gwboundary42"
	part := func(disp, content string) string {
		return "--" + b + "\r\nContent-Disposition: " + disp + "\r\n\r\n" + content + "\r\n"
	}
	evil := chatLine("evil", "gpt-evil", "x")
	for name, body := range map[string]string{
		"quoted-pair name": part(`form-data; name="purpose"`, "batch") +
			part(`form-data; name="file"; filename="a.jsonl"`, chatLine("ok", "gpt-4o", "hi")) +
			part(`form-data; name="fi\le"; filename="b.jsonl"`, evil) + "--" + b + "--\r\n",
		"rfc2231 name*": part(`form-data; name="purpose"`, "batch") +
			part(`form-data; name="xfile"; name*=UTF-8''file; filename="a.jsonl"`, chatLine("ok", "gpt-4o", "hi")) +
			part(`form-data; name="file"; name*=UTF-8''other; filename="b.jsonl"`, evil) + "--" + b + "--\r\n",
		"epilogue": part(`form-data; name="purpose"`, "batch") +
			part(`form-data; name="file"; filename="a.jsonl"`, chatLine("ok", "gpt-4o", "hi")) + "--" + b + "--\r\n" +
			part(`form-data; name="file"; filename="b.jsonl"`, evil) + "--" + b + "--\r\n",
	} {
		resp, out := h.doRaw(h.keyC, http.MethodPost, "/llm/call/primary/v1/files", "multipart/form-data; boundary="+b, []byte(body))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s: %s", name, out)
	}
	assert.Empty(t, h.api.callsTo(http.MethodPost, "/v1/files"))
}
