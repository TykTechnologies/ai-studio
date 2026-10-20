package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sigV4Verifier is a mock Bedrock endpoint that checks a request's SigV4
// signature the way AWS does: it rebuilds the request from the headers the
// Authorization header says were signed and signs it again.
func sigV4Verifier(t *testing.T, accessKey, secret, region string, got *http.Header) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		auth := r.Header.Get("Authorization")
		signed := between(auth, "SignedHeaders=", ",")
		want := between(auth+",", "Signature=", ",")
		when, err := time.Parse("20060102T150405Z", r.Header.Get("X-Amz-Date"))
		if err != nil || signed == "" || want == "" {
			http.Error(w, "unsigned", http.StatusForbidden)
			return
		}
		check, _ := http.NewRequest(r.Method, "http://"+r.Host+r.URL.RequestURI(), bytes.NewReader(body))
		for _, h := range strings.Split(signed, ";") {
			if h != "host" {
				check.Header[http.CanonicalHeaderKey(h)] = r.Header.Values(h)
			}
		}
		sum := sha256.Sum256(body)
		err = v4signer.NewSigner().SignHTTP(context.Background(), aws.Credentials{AccessKeyID: accessKey, SecretAccessKey: secret},
			check, hex.EncodeToString(sum[:]), "bedrock", region, when)
		if err != nil || between(check.Header.Get("Authorization")+",", "Signature=", ",") != want {
			http.Error(w, "signature mismatch", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	if j := strings.Index(s, end); j >= 0 {
		return s[:j]
	}
	return s
}

// The gateway adds traceparent and baggage to Bedrock passthrough requests
// before it signs them (injectUpstreamTrace, then setVendorAuthHeader, as the
// REST director and the streaming path do), so the signature covers them and
// AWS accepts the request.
func TestAgentTraceSigV4_TraceHeadersAreSignedAndAccepted(t *testing.T) {
	withW3CPropagation(t)
	withAgentTracing(t, true)
	var received http.Header
	srv := sigV4Verifier(t, "AKIDEXAMPLE", "secret", "us-east-1", &received)
	defer srv.Close()

	llm := &models.LLM{Vendor: models.BEDROCK, APIEndpoint: srv.URL, Metadata: models.JSONMap{
		"aws_access_key_id": "AKIDEXAMPLE", "aws_secret_access_key": "secret", "aws_region": "us-east-1",
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

	signed := between(received.Get("Authorization"), "SignedHeaders=", ",")
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
