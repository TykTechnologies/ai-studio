package bedrockVendor

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closingWriterTo is the request body smithy-go hands net/http: once
// closed, WriteTo answers io.EOF (q0z.26).
type closingWriterTo struct {
	mu     sync.Mutex
	r      *bytes.Reader
	closed bool
}

func (b *closingWriterTo) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.EOF
	}
	return b.r.Read(p)
}

func (b *closingWriterTo) WriteTo(w io.Writer) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, io.EOF
	}
	return b.r.WriteTo(w)
}

func (b *closingWriterTo) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

type captureDo struct{ body io.ReadCloser }

func (c *captureDo) Do(r *http.Request) (*http.Response, error) {
	c.body = r.Body
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

// The SDK's HTTP client never offers net/http a body with WriteTo, so a
// drain after smithy closed the body ends cleanly instead of failing the
// connection. Reads and Close still reach the SDK's body.
func TestSDKHTTPClientHidesWriteTo(t *testing.T) {
	base := &captureDo{}
	c := &sdkHTTPClient{base: base}
	body := &closingWriterTo{r: bytes.NewReader([]byte(`{"messages":[]}`))}
	req, err := http.NewRequest(http.MethodPost, "http://bedrock.invalid/model/m/converse-stream", body)
	require.NoError(t, err)

	_, err = c.Do(req)
	require.NoError(t, err)
	_, isWriterTo := base.body.(io.WriterTo)
	assert.False(t, isWriterTo, "the transport must not see WriteTo")

	require.NoError(t, base.body.Close())
	assert.True(t, body.closed, "Close reaches the SDK's body")
	n, err := io.Copy(io.Discard, base.body)
	assert.Zero(t, n)
	assert.NoError(t, err, "draining a closed body is a clean end, not an error")

	// What the transport did with the SDK's body as it was
	n, err = io.Copy(io.Discard, body)
	assert.Zero(t, n)
	assert.True(t, errors.Is(err, io.EOF), "unwrapped, the drain fails with io.EOF: %v", err)
}

// A Bedrock client keeps the configuration of the SDK's own HTTP client:
// with AWS_CA_BUNDLE naming the CA of a TLS fake Bedrock, Converse reaches
// it through the wrapper. LoadDefaultConfig refuses a CA bundle for a
// client it did not build, so wrapping it there broke every call.
func TestNewBedrockClientKeepsCABundle(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2}}`))
	}))
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600))
	t.Setenv("AWS_CA_BUNDLE", ca)

	llm := &models.LLM{APIEndpoint: srv.URL, Metadata: models.JSONMap{
		"aws_access_key_id": "AKIDTEST", "aws_secret_access_key": "secret", "aws_region": "us-east-1"}}
	llm.ID = 987654
	t.Cleanup(func() { InvalidateClientCache(llm.ID) })
	client, err := NewBedrockClient(llm)
	require.NoError(t, err)
	_, wrapped := client.Options().HTTPClient.(*sdkHTTPClient)
	assert.True(t, wrapped, "the SDK client sends through sdkHTTPClient")

	out, err := client.Converse(context.Background(), &bedrockruntime.ConverseInput{
		ModelId:  aws.String("m"),
		Messages: []types.Message{{Role: types.ConversationRoleUser, Content: []types.ContentBlock{&types.ContentBlockMemberText{Value: "hi"}}}},
	})
	require.NoError(t, err, "the custom CA bundle must still apply")
	require.NotNil(t, out)
}

// End to end against a server, the client sends the body and reads the
// response.
func TestSDKHTTPClientRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	body := &closingWriterTo{r: bytes.NewReader([]byte("ping"))}
	req, err := http.NewRequest(http.MethodPost, srv.URL, body)
	require.NoError(t, err)
	req.ContentLength = 4
	resp, err := newSDKHTTPClient(nil).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(got))
}
