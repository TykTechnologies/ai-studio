package openaiVendor

import (
	"bytes"
	"io"
	"mime/multipart"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A moderation batch line may omit body.model, as on the synchronous API: it
// runs on OpenAI's default, which the allow-list is checked against. Every
// other url still requires a model, whatever order the line's keys come in.
func TestBatchModerationLineDefaultsTheModel(t *testing.T) {
	for _, line := range []string{
		`{"custom_id":"m1","method":"POST","url":"/v1/moderations","body":{"input":"text"}}`,
		`{"custom_id":"m1","method":"POST","body":{"input":"text"},"url":"/v1/moderations"}`,
		`{"custom_id":"m1","method":"POST","url":"/v1/moderations","body":{"model":"","input":"text"}}`,
	} {
		items, err := splitBatchFile(uploadBody(t, "batch", line))
		require.NoError(t, err, line)
		require.Len(t, items, 1)
		assert.Equal(t, DefaultModerationModel, items[0].Model)
	}
	items, err := splitBatchFile(uploadBody(t, "batch", `{"custom_id":"m1","method":"POST","url":"/v1/moderations","body":{"model":"text-moderation-stable","input":"x"}}`))
	require.NoError(t, err)
	assert.Equal(t, "text-moderation-stable", items[0].Model, "a named model is kept")

	for _, line := range []string{
		`{"custom_id":"e1","method":"POST","url":"/v1/embeddings","body":{"input":"x"}}`,
		`{"custom_id":"r1","method":"POST","body":{"input":"x"},"url":"/v1/responses"}`,
	} {
		_, err := splitBatchFile(uploadBody(t, "batch", line))
		assert.ErrorIs(t, err, errBatchFile, line)
	}
}

func TestIsModerationsPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/v1/moderations": true, "/v1/moderations/": true, "/openai/v1/Moderations": true,
		"/v1/moderation": false, "/v1/moderations/x": false, "/v1/responses": false,
	} {
		assert.Equal(t, want, IsModerationsPath(p), p)
	}
}

// The strict multipart parser refuses quoted-pair escapes, so a file name
// with a backslash or a double quote (which Go's writer, like other
// clients, escapes) cannot be uploaded; documented under Known limits.
func TestSplitBatchFileRefusesEscapedFilenames(t *testing.T) {
	line := `{"custom_id":"a","method":"POST","url":"/v1/moderations","body":{"input":"x"}}`
	for _, name := range []string{`dir\batch.jsonl`, `my "batch".jsonl`} {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		require.NoError(t, mw.SetBoundary(testBoundary))
		require.NoError(t, mw.WriteField("purpose", "batch"))
		fw, err := mw.CreateFormFile("file", name)
		require.NoError(t, err)
		_, _ = io.WriteString(fw, line)
		require.NoError(t, mw.Close())
		_, err = splitBatchFile(b.Bytes())
		assert.ErrorIs(t, err, errBatchFile, name)
	}
	_, err := splitBatchFile(uploadBody(t, "batch", line))
	assert.NoError(t, err, "a plain name is fine")
}
