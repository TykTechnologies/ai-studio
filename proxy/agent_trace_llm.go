package proxy

import (
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
)

// goAnalyzeFor is goAnalyze for work that completes the request's agent
// tracing capture: it holds the recorder open (Recorder.Hold) until fn has
// run, since the analysis runs after the handler returned. With tracing off
// it is goAnalyze.
func (p *Proxy) goAnalyzeFor(r *http.Request, fn func()) {
	rec := agenttrace.From(r.Context())
	if rec == nil {
		p.goAnalyze(fn)
		return
	}
	release := rec.Hold()
	p.goAnalyze(func() {
		defer release()
		fn()
	})
}

// noteFirstToken records the time to first token on the request's capture.
func noteFirstToken(r *http.Request, ttft time.Duration) {
	if rec := agenttrace.From(r.Context()); rec != nil {
		rec.Update(func(ev *agenttrace.Event) {
			if ev.TTFT == 0 {
				ev.TTFT = ttft
			}
		})
	}
}

// captureLLM completes the request's llm_inference capture (spec §3.2) on
// the analytics goroutine, after the response was sent: the extraction
// library parses the bodies here, never on the request path (§10). Usage
// and cost are copied from the chat record the analytics just built, never
// computed again. The bodies go to the content pipeline through CaptureRaw;
// the capture sees them before the ProxyLog's truncation and DontLogBodies,
// because what may be kept is the content pipeline's decision.
func captureLLM(r *http.Request, llm *models.LLM, app *models.App, status int, record *models.LLMChatRecord, reqBody, respBody []byte, streamed bool) {
	rec := agenttrace.From(r.Context())
	if rec == nil || llm == nil {
		return
	}
	fp := agenttrace.CurrentFingerprinter()
	format := requestFilterFormatFor(r, llm)
	var x agenttrace.Extracts
	var serverTools int
	if streamed {
		res := captureStreamedResponse(r, llm, "", respBody, 0, fp)
		x, serverTools = res.Extracts, len(res.ServerToolCalls)
	} else if resp, err := extract.ParseResponse(format, respBody, fp); err == nil {
		resp.Apply(&x)
		serverTools = len(resp.ServerToolCalls)
	}
	if req, err := extract.ParseRequest(format, reqBody, fp); err == nil {
		req.Apply(&x)
	}
	id := authidentity.From(r.Context())
	requested := requestedModel(r)
	rec.Update(func(ev *agenttrace.Event) {
		ev.Kind = agenttrace.KindLLMInference
		ev.Provider = metrics.GenAIProviderName(string(llm.Vendor))
		ev.Operation = metrics.OperationChat
		ev.RequestModel = firstNonEmptyString(requested, llm.DefaultModel)
		ev.Resource = agenttrace.Resource{Type: agenttrace.ResourceLLM, ID: uintString(llm.ID), Name: llm.Name}
		fillAppIdentity(ev, app)
		if id != nil {
			ev.Identity.OnBehalfOf, ev.Identity.ActingAgent = id.OnBehalfOf(), id.ActingAgent()
		}
		ev.EndTime = time.Now()
		ev.HTTPStatus = status
		if status >= http.StatusBadRequest {
			ev.Status = agenttrace.StatusError
			if ev.ErrorType == "" {
				ev.ErrorType = strconv.Itoa(status)
			}
		}
		if record != nil {
			ev.ResponseModel = record.Name
			ev.Usage = usageFromRecord(record)
		}
		ev.Extracts = x
		if serverTools > 0 {
			if ev.Attributes == nil {
				ev.Attributes = map[string]string{}
			}
			ev.Attributes["tyk.server_tool_calls"] = strconv.Itoa(serverTools)
		}
	})
	rec.CaptureRaw(agenttrace.PartRequest, "application/json", reqBody)
	rec.CaptureRaw(agenttrace.PartResponse, "application/json", respBody)
}

// captureOuterCall records the /ai/ bridge's logical LLM call on the outer
// hop: the LLM and model that answered, how many rungs were tried, the
// status, and the extracts from the OpenAI-shaped exchange. The usage and
// cost belong to the inner hop that answered (its chat record); the
// assembler folds the inner hops in as attempts of this call. Bedrock rungs
// answer on this hop and fill the capture themselves (captureBedrock).
func (p *Proxy) captureOuterCall(r *http.Request, served llmAttempt, tried, status int, reqBody, respBody []byte) {
	rec := agenttrace.From(r.Context())
	if rec == nil || served.conf == nil || served.conf.Vendor == models.BEDROCK {
		if rec != nil && served.conf == nil {
			rec.Update(func(ev *agenttrace.Event) {
				ev.Kind = agenttrace.KindLLMInference
				ev.Attempt = uint32(tried)
				ev.HTTPStatus, ev.Status = status, agenttrace.StatusError
			})
		}
		return
	}
	app, _ := r.Context().Value("app").(*models.App)
	p.goAnalyzeFor(r, func() {
		fp := agenttrace.CurrentFingerprinter()
		var x agenttrace.Extracts
		if resp, err := extract.ParseResponse(requestFilterFormatOpenAI, respBody, fp); err == nil {
			resp.Apply(&x)
		}
		if req, err := extract.ParseRequest(requestFilterFormatOpenAI, reqBody, fp); err == nil {
			req.Apply(&x)
		}
		rec.Update(func(ev *agenttrace.Event) {
			ev.Kind = agenttrace.KindLLMInference
			ev.Provider = metrics.GenAIProviderName(string(served.conf.Vendor))
			ev.Operation = metrics.OperationChat
			ev.RequestModel, ev.ResponseModel = served.model, served.model
			ev.Resource = agenttrace.Resource{Type: agenttrace.ResourceLLM, ID: uintString(served.conf.ID), Name: served.conf.Name}
			fillAppIdentity(ev, app)
			ev.Attempt = uint32(tried)
			ev.HTTPStatus = status
			ev.EndTime = time.Now()
			ev.Extracts = x
		})
		rec.CaptureRaw(agenttrace.PartRequest, "application/json", reqBody)
		rec.CaptureRaw(agenttrace.PartResponse, "application/json", respBody)
	})
}

func firstNonEmptyString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
