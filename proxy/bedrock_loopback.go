package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// newBedrockLoopbackClient is the Bedrock SDK client of one bridge rung (/ai/,
// the unified /v1, routers, /anthropic/). It sends the Converse call to this
// proxy's own /llm/call/{slug} rather than to AWS, as the driver of every
// other vendor does (runDriverAttempt): the inner hop takes the hand-off of
// the outer hop's authentication, runs the plugins, the budget, the filters
// and the response hooks, signs for AWS with the LLM's keys and records the
// analytics, all against the native Converse body.
//
// The client signs nothing (anonymous credentials; the inner hop signs) and
// never retries: a retry would run the whole inner hop again, and the
// failover waterfall owns retries. It is built per request, like the drivers.
func (p *Proxy) newBedrockLoopbackClient(r *http.Request, a llmAttempt) *bedrockruntime.Client {
	// The SDK requires a region. The inner hop signs for the real one.
	region, err := bedrockVendor.Region(a.conf)
	if err != nil {
		region = "us-east-1"
	}
	internal := p.newInternalRoutingClient(r.Header.Get("Authorization"), withClientLoopbackHeaders(r, p.loopbackHeaders(r.Context(), a)))
	return bedrockruntime.New(bedrockruntime.Options{
		BaseEndpoint: aws.String(p.getInternalLLMBaseURL(a.slug, models.BEDROCK)),
		Region:       region,
		Credentials:  aws.AnonymousCredentials{},
		Retryer:      aws.NopRetryer{},
		HTTPClient:   bedrockVendor.SDKHTTPClient(converseCheckClient{internal}),
	}, func(o *bedrockruntime.Options) {
		// Without credentials the SDK would pick bearer auth next (and
		// AWS_BEARER_TOKEN_BEDROCK prefers it): ask for no auth at all. Set
		// here, after New has read the environment.
		o.AuthSchemePreference = []string{"noAuth"}
	})
}

// bedrockLoopbackContext is ctx for a loopback SDK call: it carries the
// Server-Timing recorder and the request ids across the hop, and the relay
// that keeps the inner hop's plugin headers and refusals, as
// runDriverAttempt does.
func bedrockLoopbackContext(ctx context.Context, r *http.Request, relay *loopbackRelay) context.Context {
	if rt := timingFrom(r.Context()); rt != nil {
		ctx = withRequestTiming(ctx, rt)
	}
	ctx = withRequestIDsFrom(ctx, r)
	return withLoopbackRelay(ctx, relay)
}

// maxConverseCheckBody bounds what converseCheckClient reads of a buffered
// Converse answer to check it.
const maxConverseCheckBody = 32 << 20

// converseCheckClient makes an inner-hop answer the SDK cannot read an error.
// A plugin that blocks with status 200 and its own body (or a cache hit of
// the wrong shape) answers the SDK with something that is not a Converse
// response: a buffered call would decode it into an empty answer, a stream
// would fail on its first frame. Such an answer becomes a 502 refusal, kept
// on the relay like any other, whose message is the body's own when it reads
// as an error.
type converseCheckClient struct {
	c *http.Client
}

func (c converseCheckClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.c.Do(req)
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, err
	}
	stream := strings.HasSuffix(req.URL.Path, "-stream")
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if stream {
		if strings.HasPrefix(ct, "application/vnd.amazon.eventstream") {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxRelayedErrorBody))
		_ = resp.Body.Close()
		return notConverse(req, resp, body), nil
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxConverseCheckBody))
	_ = resp.Body.Close()
	if rerr != nil {
		return nil, rerr
	}
	if isConverseOutput(body) {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return resp, nil
	}
	return notConverse(req, resp, body), nil
}

// isConverseOutput reports whether body is a Converse response: a JSON
// object with an output member.
func isConverseOutput(body []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return false
	}
	_, ok := fields["output"]
	return ok
}

// notConverse is the 502 refusal standing in for an inner-hop answer that is
// not a Converse response, recorded on the relay.
func notConverse(req *http.Request, resp *http.Response, body []byte) *http.Response {
	msg := "the gateway's inner hop did not answer with a Bedrock Converse response"
	if apiErr, ok := parseLoopbackError(resp.StatusCode, body); ok {
		msg = apiErr.Message
	}
	errBody, _ := json.Marshal(OAIErrorResponse{Error: &APIError{Message: msg, Type: "api_error"}})
	out := &http.Response{
		Status:        http.StatusText(http.StatusBadGateway),
		StatusCode:    http.StatusBadGateway,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(errBody)),
		ContentLength: int64(len(errBody)),
		Request:       req,
	}
	if relay := loopbackRelayFrom(req.Context()); relay != nil {
		relay.captureFailure(out)
	}
	return out
}
