package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway adds traceparent and baggage to Bedrock passthrough requests
// before it signs them (injectUpstreamTrace, then setVendorAuthHeader, as the
// REST director and the streaming path do), so the signature covers them and
// AWS accepts the request. The SigV4 verifier is sigv4_transport_test.go's.
func TestAgentTraceSigV4_TraceHeadersAreSignedAndAccepted(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	var received http.Header
	srv := httptest.NewServer(sigV4VerifyHandler("AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", &received))
	defer srv.Close()

	llm := &models.LLM{Vendor: models.BEDROCK, APIEndpoint: srv.URL, Metadata: models.JSONMap{
		"aws_access_key_id": "AKIDEXAMPLE", "aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "aws_region": "us-east-1",
	}}
	in, _ := http.NewRequest(http.MethodPost, "http://gw/llm/rest/b", nil)
	in.Header.Set("Traceparent", clientTraceparent)
	in.Header.Set("Baggage", "session.id=s1,user.id=leak")
	r, rec := beginRequestCapture(in)
	defer rec.Finish()

	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hi"}]}]}`)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, srv.URL+"/model/m/converse", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Baggage", in.Header.Get("Baggage")) // the client's copy, as the director has it

	injectUpstreamTrace(req.Context(), req.Header, llm)
	p := &Proxy{}
	require.NoError(t, p.setVendorAuthHeader(req, llm))

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "the SigV4 verifier rejected the request")

	signed := sigV4Between(received.Get("Authorization"), "SignedHeaders=", ",")
	assert.Contains(t, signed, "traceparent", "traceparent is covered by the signature")
	assert.Contains(t, signed, "baggage", "baggage is covered by the signature")
	assert.Equal(t, clientTraceID, traceIDOf(received.Get("Traceparent")))
	assert.Equal(t, "session.id=s1", received.Get("Baggage"))

	// The verifier is real: a signed header changed after signing fails.
	tampered, err := http.NewRequestWithContext(r.Context(), http.MethodPost, srv.URL+"/model/m/converse", bytes.NewReader(body))
	require.NoError(t, err)
	injectUpstreamTrace(tampered.Context(), tampered.Header, llm)
	require.NoError(t, p.setVendorAuthHeader(tampered, llm))
	tampered.Header.Set("Traceparent", "00-"+strings.Repeat("1", 32)+"-"+strings.Repeat("2", 16)+"-01")
	resp, err = http.DefaultClient.Do(tampered)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}
