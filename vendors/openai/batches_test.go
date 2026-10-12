package openaiVendor

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
)

const testBoundary = "gwtestboundary123"

// uploadBody builds a files.create body the way the SDKs do.
func uploadBody(t *testing.T, purpose, content string, extra ...[2]string) []byte {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	require.NoError(t, mw.SetBoundary(testBoundary))
	if purpose != "" {
		require.NoError(t, mw.WriteField("purpose", purpose))
	}
	for _, kv := range extra {
		require.NoError(t, mw.WriteField(kv[0], kv[1]))
	}
	fw, err := mw.CreateFormFile("file", "batch.jsonl")
	require.NoError(t, err)
	_, _ = io.WriteString(fw, content)
	require.NoError(t, mw.Close())
	return b.Bytes()
}

func batchInputLine(customID, model, content string) string {
	return `{"custom_id":"` + customID + `","method":"POST","url":"/v1/chat/completions","body":{"model":"` + model +
		`","messages":[{"role":"user","content":"` + content + `"}]}}`
}

func TestBatchSpecMatches(t *testing.T) {
	m, err := nativeapi.NewMatcher((&OpenAI{}).NativeAPI())
	require.NoError(t, err)
	for _, c := range []struct{ method, path, op string }{
		{http.MethodPost, "/v1/files", "openai.files.create"},
		{http.MethodGet, "/v1/files", "openai.files.list"},
		{http.MethodGet, "/v1/files/file-1", "openai.files.retrieve"},
		{http.MethodDelete, "/v1/files/file-1", "openai.files.delete"},
		{http.MethodGet, "/v1/files/file-1/content", "openai.files.content"},
		{http.MethodPost, "/v1/batches", "openai.batches.create"},
		{http.MethodGet, "/v1/batches", "openai.batches.list"},
		{http.MethodGet, "/v1/batches/batch_1", "openai.batches.retrieve"},
		{http.MethodPost, "/v1/batches/batch_1/cancel", "openai.batches.cancel"},
	} {
		got, ok := m.Match(c.method, c.path)
		require.True(t, ok, "%s %s", c.method, c.path)
		assert.Equal(t, c.op, got.Op.Name)
	}
	rt, _ := m.Spec().Resource(ResourceFile)
	assert.True(t, rt.ValidVendorID("file-abc123XYZ"))
	assert.False(t, rt.ValidVendorID("batch_1"))
	rt, _ = m.Spec().Resource(ResourceBatch)
	assert.True(t, rt.ValidVendorID("batch_abc123"))
	assert.False(t, rt.ValidVendorID("file-abc"))
}

func TestSplitJoinBatchFile(t *testing.T) {
	keep := batchInputLine("keep", "gpt-4o-mini", "hello")
	change := batchInputLine("change", "gpt-4o", "my SECRET")
	content := keep + "\n\n" + change + "\r\n" + `{"custom_id":"emb","method":"POST","url":"/v1/embeddings","body":{"model":"text-embedding-3-small","input":"x"}}`
	body := uploadBody(t, "batch", content, [2]string{"expires_after[anchor]", "created_at"})

	items, err := splitBatchFile(body)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, "keep", items[0].Key)
	assert.Equal(t, "gpt-4o-mini", items[0].Model)
	assert.Equal(t, "openai", items[0].Format)
	assert.JSONEq(t, `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`, string(items[0].Body))
	assert.Equal(t, "change", items[1].Key)
	assert.Equal(t, "text-embedding-3-small", items[2].Model)

	// Nothing changed: the upload is forwarded as it came.
	out, err := joinBatchFile(body, items)
	require.NoError(t, err)
	assert.Equal(t, body, out)

	// A filter rewrote one body (pretty-printed): only that line changes,
	// it stays one line, and the other parts survive.
	items[1].Body = []byte("{\n  \"model\": \"gpt-4o\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"my [REDACTED]\"}]\n}")
	out, err = joinBatchFile(body, items)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(out, []byte("--"+testBoundary+"\r\n")), "same boundary, no preamble")
	mr := multipart.NewReader(bytes.NewReader(out), testBoundary)
	fields := map[string]string{}
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		b, _ := io.ReadAll(p)
		fields[p.FormName()] = string(b)
	}
	assert.Equal(t, "batch", fields["purpose"])
	assert.Equal(t, "created_at", fields["expires_after[anchor]"])
	lines := strings.Split(fields["file"], "\n")
	require.Len(t, lines, 4)
	assert.Equal(t, keep, lines[0])
	assert.Equal(t, "", lines[1])
	assert.Equal(t, `{"custom_id":"change","method":"POST","url":"/v1/chat/completions","body":{"model":"gpt-4o","messages":[{"role":"user","content":"my [REDACTED]"}]}}`+"\r", lines[2])
	assert.Contains(t, lines[3], `"custom_id":"emb"`)
	again, err := splitBatchFile(out)
	require.NoError(t, err)
	assert.Len(t, again, 3)
}

func TestSplitBatchFileRejects(t *testing.T) {
	line := batchInputLine("a", "gpt-4o", "hi")
	qp := strings.Replace(string(uploadBody(t, "batch", line)), "Content-Type: application/octet-stream",
		"Content-Type: application/octet-stream\r\nContent-Transfer-Encoding: quoted-printable", 1)
	twoFiles := func() []byte {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		_ = mw.SetBoundary(testBoundary)
		_ = mw.WriteField("purpose", "batch")
		for _, c := range []string{line, batchInputLine("b", "evil", "x")} {
			fw, _ := mw.CreateFormFile("file", "f.jsonl")
			_, _ = io.WriteString(fw, c)
		}
		_ = mw.Close()
		return b.Bytes()
	}
	noDisposition := func() []byte {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		_ = mw.SetBoundary(testBoundary)
		w, _ := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain"}})
		_, _ = io.WriteString(w, "batch")
		_ = mw.Close()
		return b.Bytes()
	}
	for name, body := range map[string][]byte{
		"not multipart":      []byte(line),
		"preamble":           append([]byte("preamble\r\n"), uploadBody(t, "batch", line)...),
		"other purpose":      uploadBody(t, "fine-tune", line),
		"no purpose":         uploadBody(t, "", line),
		"two file parts":     twoFiles(),
		"no disposition":     noDisposition(),
		"quoted-printable":   []byte(qp),
		"empty file":         uploadBody(t, "batch", "\n\n"),
		"not json":           uploadBody(t, "batch", "nope"),
		"get method":         uploadBody(t, "batch", strings.Replace(line, `"POST"`, `"GET"`, 1)),
		"unsupported url":    uploadBody(t, "batch", strings.Replace(line, "/v1/chat/completions", "/v1/files", 1)),
		"missing model":      uploadBody(t, "batch", `{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"messages":[]}}`),
		"missing custom_id":  uploadBody(t, "batch", `{"method":"POST","url":"/v1/chat/completions","body":{"model":"m"}}`),
		"missing body":       uploadBody(t, "batch", `{"custom_id":"a","method":"POST","url":"/v1/chat/completions"}`),
		"duplicate model":    uploadBody(t, "batch", `{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"ok","model":"evil"}}`),
		"case-variant body":  uploadBody(t, "batch", `{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"ok"},"Body":{"model":"evil"}}`),
		"nested duplicate":   uploadBody(t, "batch", `{"custom_id":"a","method":"POST","url":"/v1/chat/completions","body":{"model":"ok","messages":[{"role":"user","content":"a","content":"b"}]}}`),
		"second line broken": uploadBody(t, "batch", line+"\n{"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := splitBatchFile(body)
			require.Error(t, err)
			assert.ErrorIs(t, err, errBatchFile)
		})
	}
}

func TestBatchAsync(t *testing.T) {
	rt, _ := nativeSpec.Resource(ResourceBatch)
	a := rt.Async
	batch := func(status string, output any) []byte {
		b, _ := json.Marshal(map[string]any{"id": "batch_1", "object": "batch", "status": status, "created_at": 1760000000,
			"completed_at": 1760000600, "output_file_id": output})
		return b
	}
	for status, want := range map[string]nativeapi.ResourceState{
		"validating": nativeapi.StateInProgress, "in_progress": nativeapi.StateInProgress, "finalizing": nativeapi.StateInProgress,
		"cancelling": nativeapi.StateCanceling,
		"completed":  nativeapi.StateEnded, "failed": nativeapi.StateEnded, "expired": nativeapi.StateEnded, "cancelled": nativeapi.StateEnded,
	} {
		info, err := a.State(batch(status, nil))
		require.NoError(t, err)
		assert.Equal(t, want, info.State, status)
		require.NotNil(t, info.ResultsExpireAt)
		assert.Equal(t, time.Unix(1760000000, 0).UTC().Add(30*24*time.Hour), *info.ResultsExpireAt)
	}
	info, _ := a.State(batch("completed", "file-out"))
	require.NotNil(t, info.EndedAt)
	assert.Equal(t, time.Unix(1760000600, 0).UTC(), *info.EndedAt)
	_, err := a.State([]byte(`{"object":"list"}`))
	assert.Error(t, err)

	req, ok := a.Results("batch_1", batch("completed", "file-out"))
	require.True(t, ok)
	assert.Equal(t, nativeapi.UpstreamReq{Method: http.MethodGet, Path: "/v1/files/file-out/content"}, req)
	_, ok = a.Results("batch_1", batch("failed", nil))
	assert.False(t, ok, "no output file, nothing to read")
	_, ok = a.Results("batch_1", batch("completed", "../../v1/x"))
	assert.False(t, ok, "an output file ID outside the charset is never followed")
	assert.Equal(t, nativeapi.UpstreamReq{Method: http.MethodPost, Path: "/v1/batches/batch_1/cancel"}, a.Cancel("batch_1"))
}

func TestViewBatchLine(t *testing.T) {
	chat := `{"id":"batch_req_1","custom_id":"a","response":{"status_code":200,"request_id":"req_1","body":` +
		`{"model":"gpt-4o-2024-08-06","choices":[{"message":{"role":"assistant","content":"hi"}}],` +
		`"usage":{"prompt_tokens":100,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":40}}}},"error":null}`
	v, err := viewBatchLine([]byte(chat))
	require.NoError(t, err)
	assert.Equal(t, "a", v.Key)
	assert.Equal(t, "gpt-4o-2024-08-06", v.Model)
	assert.NotNil(t, v.Message)
	assert.Equal(t, nativeapi.Usage{Model: "gpt-4o-2024-08-06", InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 40,
		Tier: "batch", Multiplier: 0.5, Billed: true}, v.Usage)
	batchRT, _ := nativeSpec.Resource(ResourceBatch)
	u, ok, err := batchRT.Async.LineUsage([]byte(chat))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, v.Usage, u)

	resp := `{"id":"r","custom_id":"b","response":{"status_code":200,"body":{"object":"response","model":"gpt-4o",` +
		`"usage":{"input_tokens":7,"output_tokens":3,"input_tokens_details":{"cached_tokens":1}}}}}`
	v, err = viewBatchLine([]byte(resp))
	require.NoError(t, err)
	assert.Equal(t, 7, v.Usage.InputTokens)
	assert.Equal(t, 1, v.Usage.CacheReadInputTokens)

	emb := `{"id":"r","custom_id":"c","response":{"status_code":200,"body":{"object":"list","model":"text-embedding-3-small","usage":{"prompt_tokens":5,"total_tokens":5}}}}`
	v, err = viewBatchLine([]byte(emb))
	require.NoError(t, err)
	assert.Equal(t, 5, v.Usage.InputTokens)
	assert.True(t, v.Usage.Billed)

	for _, l := range []string{
		`{"id":"r","custom_id":"d","response":{"status_code":400,"body":{"error":{"message":"bad"}}},"error":null}`,
		`{"id":"r","custom_id":"e","response":null,"error":{"code":"batch_expired","message":"expired"}}`,
	} {
		v, err = viewBatchLine([]byte(l))
		require.NoError(t, err)
		assert.False(t, v.Usage.Billed, l)
		assert.Nil(t, v.Message, l)
	}
	_, err = viewBatchLine([]byte(`{"custom_id":""}`))
	assert.Error(t, err)

	blocked, err := blockedBatchLine([]byte(chat), "blocked by policy")
	require.NoError(t, err)
	var b struct {
		ID       string `json:"id"`
		CustomID string `json:"custom_id"`
		Response struct {
			StatusCode int    `json:"status_code"`
			RequestID  string `json:"request_id"`
			Body       struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"body"`
		} `json:"response"`
	}
	require.NoError(t, json.Unmarshal(blocked, &b))
	assert.Equal(t, "batch_req_1", b.ID)
	assert.Equal(t, "a", b.CustomID)
	assert.Equal(t, 400, b.Response.StatusCode)
	assert.Equal(t, "req_1", b.Response.RequestID)
	assert.Equal(t, "blocked by policy", b.Response.Body.Error.Message)
	v, _ = viewBatchLine(blocked)
	assert.False(t, v.Usage.Billed, "a blocked line is not billed twice")
}

// rawUpload builds a multipart body from raw part header blocks and contents.
func rawUpload(parts ...[2]string) []byte {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString("--" + testBoundary + "\r\n" + p[0] + "\r\n\r\n" + p[1] + "\r\n")
	}
	b.WriteString("--" + testBoundary + "--\r\n")
	return []byte(b.String())
}

// Parts a vendor's parser could name differently from the gateway's are
// refused, so no unchecked part can become the vendor's file.
func TestSplitBatchFileRefusesAmbiguousParts(t *testing.T) {
	benign := batchInputLine("ok", "gpt-4o", "hi")
	evil := batchInputLine("evil", "gpt-evil", "x")
	purpose := [2]string{`Content-Disposition: form-data; name="purpose"`, "batch"}
	file := [2]string{"Content-Disposition: form-data; name=\"file\"; filename=\"a.jsonl\"\r\nContent-Type: application/jsonl", benign}

	// The plain SDK shape is accepted.
	items, err := splitBatchFile(rawUpload(purpose, file))
	require.NoError(t, err)
	require.Len(t, items, 1)

	for name, body := range map[string][]byte{
		"quoted-pair name":      rawUpload(purpose, file, [2]string{`Content-Disposition: form-data; name="fi\le"; filename="b.jsonl"`, evil}),
		"rfc2231 name*":         rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="xfile"; name*=UTF-8''file; filename="a.jsonl"`, benign}),
		"rfc2231 filename*":     rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="file"; filename*=UTF-8''a.jsonl`, benign}),
		"repeated name":         rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="file"; name="other"; filename="a.jsonl"`, benign}),
		"case-variant name":     rawUpload(purpose, file, [2]string{`Content-Disposition: form-data; name="File"; filename="b.jsonl"`, evil}),
		"unknown field":         rawUpload(purpose, file, [2]string{`Content-Disposition: form-data; name="extra"`, "x"}),
		"filename on purpose":   rawUpload([2]string{`Content-Disposition: form-data; name="purpose"; filename="p"`, "batch"}, file),
		"file without filename": rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="file"`, benign}),
		"uppercase form-data":   rawUpload(purpose, [2]string{`Content-Disposition: Form-Data; name="file"; filename="a.jsonl"`, benign}),
		"two dispositions":      rawUpload(purpose, [2]string{"Content-Disposition: form-data; name=\"file\"; filename=\"a.jsonl\"\r\nContent-Disposition: form-data; name=\"other\"", benign}),
		"extra header":          rawUpload(purpose, [2]string{"Content-Disposition: form-data; name=\"file\"; filename=\"a.jsonl\"\r\nX-Part: 1", benign}),
		"epilogue part": append(rawUpload(purpose, file),
			[]byte("--"+testBoundary+"\r\nContent-Disposition: form-data; name=\"file\"; filename=\"b.jsonl\"\r\n\r\n"+evil+"\r\n--"+testBoundary+"--\r\n")...),
		"epilogue text":     append(rawUpload(purpose, file), []byte("trailing")...),
		"no closing":        []byte("--" + testBoundary + "\r\nContent-Disposition: form-data; name=\"purpose\"\r\n\r\nbatch\r\n"),
		"quote in filename": rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="file"; filename="a\".jsonl"`, benign}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := splitBatchFile(body)
			require.Error(t, err)
			assert.ErrorIs(t, err, errBatchFile)
		})
	}

	// The documented optional fields are accepted.
	_, err = splitBatchFile(rawUpload(purpose, [2]string{`Content-Disposition: form-data; name="expires_after[anchor]"`, "created_at"},
		[2]string{`Content-Disposition: form-data; name="expires_after[seconds]"`, "3600"}, file))
	require.NoError(t, err)
}
