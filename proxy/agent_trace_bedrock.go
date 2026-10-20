package proxy

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace/extract"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// bedrockTrace is what a Bedrock path captures for agent tracing: the
// tool calls the model asked for (the SDK paths used to keep only the text),
// the time to first token and the body the client got. It exists only while
// agent tracing is on; every method is a no-op on nil, so the Bedrock paths
// pay nothing with tracing off.
//
// The capture finishes on the analytics goroutine, after the handler has
// returned, so it holds the request's recorder open (Recorder.Hold) until
// captureBedrock, or until the path gives up (abandon).
type bedrockTrace struct {
	rec       *agenttrace.Recorder
	release   func()
	reqFormat string // the request body's filter format (openai, anthropic, bedrock)
	start     time.Time
	ttft      time.Duration

	mu        sync.Mutex
	calls     []*bedrockCall
	byIndex   map[int32]*bedrockCall
	argBytes  int
	stop      string
	complete  bool
	extracts  *agenttrace.Extracts // set for paths that capture in one go
	response  []byte
	handedOff bool
}

type bedrockCall struct {
	id, name    string
	args        strings.Builder
	unavailable bool
}

// bedrockMaxArgs caps argument bytes kept per streamed response (spec §4.5).
const bedrockMaxArgs = 1 << 20

func newBedrockTrace(r *http.Request, reqFormat string) *bedrockTrace {
	rec := agenttrace.From(r.Context())
	if rec == nil {
		return nil
	}
	return &bedrockTrace{rec: rec, release: rec.Hold(), reqFormat: reqFormat, start: time.Now(), byIndex: map[int32]*bedrockCall{}}
}

// firstToken notes the time to first token (once).
func (bt *bedrockTrace) firstToken() {
	if bt == nil || bt.ttft != 0 {
		return
	}
	bt.ttft = time.Since(bt.start)
}

// observe reads one Converse stream event (the SDK paths).
func (bt *bedrockTrace) observe(event types.ConverseStreamOutput) {
	if bt == nil {
		return
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		if tu, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
			c := &bedrockCall{id: aws.ToString(tu.Value.ToolUseId), name: aws.ToString(tu.Value.Name)}
			bt.calls = append(bt.calls, c)
			bt.byIndex[aws.ToInt32(v.Value.ContentBlockIndex)] = c
		}
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		if d, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
			if c := bt.byIndex[aws.ToInt32(v.Value.ContentBlockIndex)]; c != nil {
				frag := aws.ToString(d.Value.Input)
				if bt.argBytes+len(frag) > bedrockMaxArgs {
					c.unavailable = true
					return
				}
				bt.argBytes += len(frag)
				c.args.WriteString(frag)
			}
		}
	case *types.ConverseStreamOutputMemberMessageStop:
		bt.stop = string(v.Value.StopReason)
		bt.complete = true
	}
}

// setOutput reads a non-streamed Converse output.
func (bt *bedrockTrace) setOutput(output *bedrockruntime.ConverseOutput) {
	if bt == nil || output == nil {
		return
	}
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.complete = true
	bt.stop = string(output.StopReason)
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return
	}
	for _, block := range msg.Value.Content {
		tu, ok := block.(*types.ContentBlockMemberToolUse)
		if !ok {
			continue
		}
		c := &bedrockCall{id: aws.ToString(tu.Value.ToolUseId), name: aws.ToString(tu.Value.Name)}
		if tu.Value.Input != nil {
			if raw, err := tu.Value.Input.MarshalSmithyDocument(); err == nil {
				c.args.Write(raw)
			} else {
				c.unavailable = true
			}
		}
		bt.calls = append(bt.calls, c)
	}
}

// setExtracts sets extracts captured elsewhere (the native stream's
// streamCapture), in place of the SDK events.
func (bt *bedrockTrace) setExtracts(x agenttrace.Extracts) {
	if bt == nil {
		return
	}
	bt.mu.Lock()
	bt.extracts = &x
	bt.mu.Unlock()
}

// setResponse keeps the body the client got, for the content pipeline.
func (bt *bedrockTrace) setResponse(body []byte) {
	if bt == nil {
		return
	}
	bt.mu.Lock()
	bt.response = body
	bt.mu.Unlock()
}

// handOff marks the capture as owned by the analytics goroutine, which
// calls captureBedrock; abandon then does nothing.
func (bt *bedrockTrace) handOff() {
	if bt == nil {
		return
	}
	bt.mu.Lock()
	bt.handedOff = true
	bt.mu.Unlock()
}

// abandon releases the recorder when the path ends without handing the
// capture off (an error, a block). Safe to defer.
func (bt *bedrockTrace) abandon() {
	if bt == nil {
		return
	}
	bt.mu.Lock()
	owned := bt.handedOff
	bt.mu.Unlock()
	if !owned {
		bt.release()
	}
}

// responseExtracts turns what was observed into the response-side extracts.
func (bt *bedrockTrace) responseExtracts(fp agenttrace.Fingerprinter) agenttrace.Extracts {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	if bt.extracts != nil {
		return *bt.extracts
	}
	var x agenttrace.Extracts
	if bt.stop != "" {
		x.FinishReasons = []string{bt.stop}
	}
	if !bt.complete {
		x.StreamIncomplete = true
		x.FinishReasons = append(x.FinishReasons, "error")
	}
	for i, c := range bt.calls {
		if i == extract.MaxToolCalls {
			break
		}
		tc := agenttrace.ToolCallRequest{ID: boundStreamID(c.id), Name: boundStreamText(c.name, extract.MaxNameBytes)}
		if c.unavailable || !bt.complete {
			tc.ArgsUnavailable = true
		} else {
			args := c.args.String()
			if args == "" {
				args = "{}"
			}
			if f, err := extract.ArgsFingerprint(fp, []byte(args)); err == nil {
				tc.ArgsFP = f
			}
		}
		x.ToolCalls = append(x.ToolCalls, tc)
	}
	return x
}

// captureBedrock completes the request's llm_inference capture for a
// Bedrock call, on the analytics goroutine: the usage and cost come from
// the chat record the analytics just built (never computed twice), the
// request-side extracts from the request body, and the bodies go to the
// content pipeline. It releases the recorder.
func captureBedrock(r *http.Request, llm *models.LLM, app *models.App, modelID string, reqBody []byte,
	proxyLog *models.ProxyLog, record *models.LLMChatRecord, bt *bedrockTrace) {
	if bt == nil {
		return
	}
	defer bt.release()
	fp := agenttrace.CurrentFingerprinter()
	x := bt.responseExtracts(fp)
	x.Stream = x.Stream || bt.ttft > 0
	if req, err := extract.ParseRequest(bt.reqFormat, reqBody, fp); err == nil {
		req.Apply(&x)
	}
	status := http.StatusOK
	if proxyLog != nil && proxyLog.ResponseCode != 0 {
		status = proxyLog.ResponseCode
	}
	id := authidentity.From(r.Context())
	bt.rec.Update(func(ev *agenttrace.Event) {
		ev.Kind = agenttrace.KindLLMInference
		ev.Provider = metrics.GenAIProviderName(string(llm.Vendor))
		ev.Operation = metrics.OperationChat
		ev.RequestModel, ev.ResponseModel = modelID, modelID
		ev.Resource = agenttrace.Resource{Type: agenttrace.ResourceLLM, ID: uintString(llm.ID), Name: llm.Name}
		fillAppIdentity(ev, app)
		if id != nil {
			ev.Identity.OnBehalfOf, ev.Identity.ActingAgent = id.OnBehalfOf(), id.ActingAgent()
		}
		ev.EndTime = time.Now()
		ev.TTFT = bt.ttft
		ev.HTTPStatus = status
		if status >= http.StatusBadRequest {
			ev.Status = agenttrace.StatusError
		}
		if record != nil {
			ev.Usage = usageFromRecord(record)
		}
		ev.Extracts = x
	})
	bt.rec.CaptureRaw(agenttrace.PartRequest, "application/json", reqBody)
	bt.mu.Lock()
	resp := bt.response
	bt.mu.Unlock()
	bt.rec.CaptureRaw(agenttrace.PartResponse, "application/json", resp)
}

// usageFromRecord copies the chat record's usage and cost (Cost is in
// currency units x 10,000; the capture holds micros).
func usageFromRecord(rec *models.LLMChatRecord) agenttrace.Usage {
	return agenttrace.Usage{
		InputTokens:      int64(rec.PromptTokens),
		OutputTokens:     int64(rec.ResponseTokens),
		CacheReadTokens:  int64(rec.CacheReadPromptTokens),
		CacheWriteTokens: int64(rec.CacheWritePromptTokens),
		CostMicros:       int64(rec.Cost * 100),
		Currency:         rec.Currency,
	}
}

// fillAppIdentity stamps the App, its team, owner and credential.
func fillAppIdentity(ev *agenttrace.Event, app *models.App) {
	if app == nil {
		return
	}
	ev.Identity.AppID = app.ID
	ev.Identity.UserID = app.UserID
	if app.TeamID != nil {
		ev.Identity.TeamID = *app.TeamID
	}
	if app.CredentialID != 0 {
		ev.Identity.CredentialID = uintString(app.CredentialID)
	}
}

func uintString(v uint) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(v), 10)
}

// bedrockMetrics counts a Bedrock bridge request in flight and observes its
// duration when the returned func is called with the status the client saw.
// The bridges call Bedrock directly, so the /llm/ handlers that record these
// never run for them.
func bedrockMetrics(r *http.Request, llm *models.LLM, streaming bool) func(status int) {
	start := time.Now()
	metrics.IncrementInflight(r.Context(), string(llm.Vendor))
	return func(status int) {
		metrics.DecrementInflight(r.Context(), string(llm.Vendor))
		metrics.ObserveRequestDuration(r.Context(), string(llm.Vendor), llm.Name, streaming, time.Since(start).Seconds(), status)
	}
}

// statusOr is the status a failed attempt answered with, or ok when it did
// not fail.
func (f attemptFailure) statusOr(ok int) int {
	switch {
	case f.err == nil:
		return ok
	case f.status != 0:
		return f.status
	}
	return http.StatusBadGateway
}

// statusWriter records the status a handler wrote.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
