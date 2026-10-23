package proxy

import (
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// streamCompletion sends a streamed request's OnStreamComplete exactly once,
// however the stream ends: completed, an upstream error response, a filter
// or guardrail block, or a transport error with no response. Plugins that
// hold per-request state from post_auth (a load balancer's or rate
// limiter's in-flight count, a cache's pending entry) release it there, so
// a stream that ends without one leaks it (bpu).
type streamCompletion struct {
	p       *Proxy
	r       *http.Request
	llm     *models.LLM
	app     *models.App
	reqBody []byte
	sent    bool
}

func (p *Proxy) newStreamCompletion(r *http.Request, llm *models.LLM, app *models.App, reqBody []byte) *streamCompletion {
	return &streamCompletion{p: p, r: r, llm: llm, app: app, reqBody: reqBody}
}

// complete sends the completion with the status the stream ended with and
// what it accumulated; later calls do nothing. A stream that did not end
// normally carries a non-2xx status (400 for a block, the transport error's
// 502/504), so caches never store it.
func (c *streamCompletion) complete(status int, header http.Header, accumulated []byte, chunkCount int) {
	if c.sent {
		return
	}
	c.sent = true
	if c.p.responseHookManager == nil || !c.p.hasResponseHooks() {
		return
	}
	resp := &http.Response{StatusCode: status, Header: header}
	go c.p.executeOnStreamComplete(c.r, resp, c.llm, c.app, accumulated, c.reqBody, chunkCount)
}
