package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/metrics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/agenttrace"
	"github.com/TykTechnologies/midsommar/v2/switches"
	bedrockVendor "github.com/TykTechnologies/midsommar/v2/vendors/bedrock"
	"github.com/rs/zerolog/log"
)

// handleBedrockStreamingProxy serves a native Bedrock stream on /llm/stream/
// (converse-stream, and invoke-with-response-stream when the LLM has no
// response filters). Like the native REST path it forwards the request body
// as the client wrote it, signed with SigV4, and relays the AWS binary event
// stream frame by frame, byte for byte: tools, tool results, images,
// documents, reasoning and every other block reach the vendor and the
// client unchanged. The frames are decoded only to read them: the text for
// response filters, tool use (held until classified when a response
// guardrail scans tool calls) and the usage for analytics. A block ends the
// stream with a validationException message, which the AWS SDKs raise.
// Response hooks run as on every other stream: the header phase before the
// status is written, OnStreamComplete once the stream ends. OnStreamComplete
// gets the events as SSE data: lines (converseEventLine), the text form
// plugins read every other vendor's stream in.
func (p *Proxy) handleBedrockStreamingProxy(w http.ResponseWriter, r *http.Request, llm *models.LLM, app *models.App, reqBody []byte) {
	startTime := time.Now()
	hasResponseFilters := p.hasResponseFilters(llm)

	lower := strings.ToLower(r.URL.Path)
	op := ""
	switch {
	case strings.Contains(lower, "converse-stream"):
		op = "converse-stream"
	case bedrockInvokeStream(lower):
		if hasResponseFilters {
			respondWithError(w, http.StatusBadRequest, "Bedrock invoke-with-response-stream cannot be screened by this LLM's response filters; use converse-stream", nil, false)
			return
		}
		op = "invoke-with-response-stream"
	case strings.HasSuffix(strings.TrimSuffix(lower, "/"), "/converse"), strings.HasSuffix(strings.TrimSuffix(lower, "/"), "/invoke"):
		respondWithError(w, http.StatusBadRequest, "this is the streaming route; call converse-stream (or use /llm/rest for converse)", nil, false)
		return
	default:
		// A body asking to stream ({"stream": true}) on a path that names no
		// operation is a Converse request.
		op = "converse-stream"
	}

	// The model: from the path (native clients send /model/{id}/...), then
	// a modelId in the body, then the LLM's default. modelId is not a
	// Converse field, so it is taken out of what is forwarded.
	body := reqBody
	modelID := ExtractBedrockModelIDFromPath(r.URL.EscapedPath())
	var fields map[string]json.RawMessage
	if json.Unmarshal(reqBody, &fields) == nil {
		if raw, ok := fields["modelId"]; ok {
			var fromBody string
			_ = json.Unmarshal(raw, &fromBody)
			if modelID == "" {
				modelID = fromBody
			}
			delete(fields, "modelId")
			if b, err := json.Marshal(fields); err == nil {
				body = b
			}
		}
	} else if op == "converse-stream" {
		respondWithError(w, http.StatusBadRequest, "invalid Bedrock Converse request body", nil, false)
		return
	}
	if modelID == "" {
		modelID = llm.DefaultModel
	}
	if modelID == "" {
		respondWithError(w, http.StatusBadRequest, "model ID is required (set modelId in request body or configure DefaultModel)", nil, false)
		return
	}

	// A plugin's upstream_override (a load balancer's pick) replaces the
	// LLM's endpoint, as on /llm/rest/; SigV4 signs for the region it names.
	base, err := bedrockVendor.RuntimeBaseURL(llm)
	if override, ok := r.Context().Value("upstream_override").(string); ok && override != "" {
		log.Debug().Str("override", override).Msg("Using upstream override from plugin for Bedrock streaming")
		base, err = bedrockVendor.RuntimeBaseURLFor(override)
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid Bedrock endpoint", err, false)
		return
	}
	upstream, err := url.Parse(base + "/model/" + url.PathEscape(modelID) + "/" + op)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "invalid Bedrock endpoint", err, false)
		return
	}
	exempt := llmExemption(llm)
	if err := exempt.ValidateURL(upstream); err != nil {
		log.Error().Err(err).Msg("Bedrock streaming upstream URL blocked by SSRF policy")
		respondWithError(w, http.StatusBadGateway, upstreamErrorMessage("upstream endpoint not permitted", err), err, false)
		return
	}
	upReq, err := http.NewRequestWithContext(exempt.WithContext(r.Context()), http.MethodPost, upstream.String(), bytes.NewReader(body))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to create Bedrock request", err, false)
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "application/vnd.amazon.eventstream")
	// Trace context first: SigV4 then signs it (see agent_trace_sigv4_test).
	injectUpstreamTrace(upReq.Context(), upReq.Header, llm)
	if err := switches.SetVendorAuthHeader(upReq, llm); err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to sign Bedrock request", err, false)
		return
	}
	// From here every exit completes the stream for the response hooks
	// (streamCompletion); the last resort is a broken stream's 502.
	sc := p.newStreamCompletion(r, llm, app, reqBody)
	defer sc.complete(http.StatusBadGateway, nil, nil, 0)
	resp, err := (&http.Client{Timeout: p.config.llmTimeout(), Transport: upstreamTransport}).Do(upReq)
	if err != nil {
		// A vendor that cannot be reached answers 502 (a timeout 504) and
		// leaves its row, as on every other stream.
		status := loopbackAwareTransportStatus(r, err)
		p.goAnalyzeFor(r, func() {
			recordBedrockFailedAttempt(llm, app, modelID, reqBody, status, upstreamFailureBody(status, err), r, startTime)
		})
		respondWithError(w, status, "Bedrock stream request failed", err, false)
		sc.complete(status, nil, nil, 0)
		return
	}
	defer resp.Body.Close()

	hooks := p.responseHookManager != nil && p.hasResponseHooks()
	if resp.StatusCode != http.StatusOK {
		// The vendor's error (JSON) goes to the client as it came.
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		if hooks {
			p.executeStreamingHeaderHooks(w.Header(), r, llm, app)
		}
		w.WriteHeader(resp.StatusCode)
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_, _ = w.Write(errBody)
		p.goAnalyzeFor(r, func() {
			recordBedrockFailedAttempt(llm, app, modelID, reqBody, resp.StatusCode, errBody, r, startTime)
		})
		// As on every other stream, an upstream error completes the stream
		// too: plugins that hold per-request state (a load balancer's or rate
		// limiter's in-flight count, a cache's pending entry) release it here.
		sc.complete(resp.StatusCode, resp.Header, errBody, 0)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
	w.Header().Set("X-Accel-Buffering", "no")
	if hooks {
		p.executeStreamingHeaderHooks(w.Header(), r, llm, app)
	}
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	flush()

	dialect := streamDialect("bedrock")
	framer := newEventFramer("application/vnd.amazon.eventstream")
	var hold *toolCallHold
	if hasResponseFilters && responseFiltersScanToolCalls(llm) {
		hold = newToolCallHold(dialect)
	}
	var textBuffer strings.Builder
	bt := newBedrockTrace(r, "bedrock")
	defer bt.abandon()
	var capture *streamCapture
	if bt != nil {
		capture = newStreamCapture("application/vnd.amazon.eventstream", dialect, 0, agenttrace.CurrentFingerprinter())
	}
	ttftSeen := false
	var inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens int
	usageFrom := func(ev streamEvent) {
		if ev.name != "metadata" {
			return
		}
		var m struct {
			Usage struct {
				InputTokens           int `json:"inputTokens"`
				OutputTokens          int `json:"outputTokens"`
				CacheWriteInputTokens int `json:"cacheWriteInputTokens"`
				CacheReadInputTokens  int `json:"cacheReadInputTokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(ev.data, &m) == nil {
			inputTokens, outputTokens = m.Usage.InputTokens, m.Usage.OutputTokens
			cacheWriteTokens, cacheReadTokens = m.Usage.CacheWriteInputTokens, m.Usage.CacheReadInputTokens
		}
	}
	buf := make([]byte, 32<<10)
	// The events as the vendor sent them, as data: lines, for OnStreamComplete
	var full bytes.Buffer
	chunkIndex := 0
	// completeBlocked completes a stream a filter ended, with the 400 its
	// analytics records.
	completeBlocked := func() { sc.complete(http.StatusBadRequest, resp.Header, full.Bytes(), chunkIndex) }
	block := func(msg string) {
		recordPolicyBlock(r.Context(), "response_filter", "filter")
		log.Warn().Str("reason", msg).Msg("Bedrock stream blocked by filter")
		markStreamBlocked(w, r, msg)
		_, _ = w.Write(dialect.blockEvent(framer, msg))
		flush()
		completeBlocked()
	}
	isErr, clientGone := false, false
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if !ttftSeen {
				ttftSeen = true
				bt.firstToken()
				metrics.ObserveTimeToFirstToken(r.Context(), string(llm.Vendor), llm.Name, time.Since(startTime).Seconds())
			}
			if capture != nil {
				capture.Feed(chunk)
			}
			events := framer.feed(chunk)
			var delta strings.Builder
			var release []byte
			for _, ev := range events {
				usageFrom(ev)
				if hooks {
					full.Write(converseEventLine(ev))
				}
				t := dialect.text(ev)
				delta.WriteString(t)
				if hold == nil {
					release = append(release, ev.raw...)
					continue
				}
				now, ready := hold.observe(ev)
				release = append(release, now...)
				if !ready {
					continue
				}
				held, calls, overflow := hold.take()
				blocked, msg, ferr := classifyHeldCalls(llm, p.gatewayService, calls, overflow, body, "bedrock", http.StatusOK, r)
				if ferr != nil {
					log.Error().Err(ferr).Msg("Tool call filter error on a Bedrock stream")
				}
				if blocked {
					_, _ = w.Write(release)
					block(msg)
					return
				}
				release = append(release, held...)
			}
			textBuffer.WriteString(delta.String())
			if hasResponseFilters && len(events) > 0 {
				blocked, msg, ferr := ExecuteStreamTextFilters(llm, p.gatewayService, delta.String(), http.StatusOK, chunkIndex, textBuffer.String(), r)
				if ferr != nil {
					log.Error().Err(ferr).Int("chunk", chunkIndex).Msg("Response filter error on Bedrock stream chunk")
				} else if blocked {
					block(msg)
					return
				}
			}
			if !hasResponseFilters {
				// Nothing to screen: relay the read as it came.
				release = chunk
			}
			if len(release) > 0 {
				if _, werr := w.Write(release); werr != nil {
					clientGone = true
					break
				}
				flush()
				chunkIndex++
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if r.Context().Err() != nil {
				clientGone = true
			} else {
				log.Error().Err(readErr).Msg("Bedrock stream error")
				isErr = true
			}
			break
		}
	}

	// What the framer still holds (a frame cut short) is relayed only if the
	// filters pass; held calls are classified first.
	var tail []byte
	if hasResponseFilters {
		if ev := framer.flush(); ev != nil {
			tail = ev.raw
		}
		if hold != nil && hold.holding {
			held, calls, overflow := hold.take()
			if !isErr {
				blocked, msg, ferr := classifyHeldCalls(llm, p.gatewayService, calls, overflow, body, "bedrock", http.StatusOK, r)
				if ferr != nil {
					log.Error().Err(ferr).Msg("Tool call filter error at the end of a Bedrock stream")
				}
				if blocked {
					if !clientGone {
						block(msg)
					}
					completeBlocked()
					return
				}
				tail = append(held, tail...)
			}
		}
		if !isErr {
			blocked, msg, ferr := ExecuteFinalResponseFilters(llm, p.gatewayService, http.StatusOK, chunkIndex, textBuffer.String(), r)
			if ferr != nil {
				log.Error().Err(ferr).Msg("Response filter error at the end of a Bedrock stream")
			} else if blocked {
				if !clientGone {
					block(msg)
				}
				completeBlocked()
				return
			}
		}
		if len(tail) > 0 && !clientGone {
			_, _ = w.Write(tail)
			flush()
		}
	}

	if !isErr && (inputTokens > 0 || outputTokens > 0) {
		responseText := textBuffer.String()
		if capture != nil {
			bt.setExtracts(capture.Finish().Extracts)
		}
		bt.handOff()
		go func() {
			recordBedrockStreamAnalytics(p, llm, app, modelID, reqBody, responseText, inputTokens, outputTokens, cacheWriteTokens, cacheReadTokens, r, startTime, bt)
		}()
	}
	if isErr {
		// The upstream broke off mid-stream: the client has a truncated
		// stream, which no cache may store.
		sc.complete(http.StatusBadGateway, resp.Header, full.Bytes(), chunkIndex)
		return
	}
	sc.complete(resp.StatusCode, resp.Header, full.Bytes(), chunkIndex)
}

// converseEventLine is a Converse stream event as OnStreamComplete gets it:
// an SSE data: line holding the event in the API's JSON union form, the AWS
// wire JSON keyed by its event type ({"contentBlockDelta":{"delta":{"text":
// "Hi"},"contentBlockIndex":0}}, {"metadata":{"usage":{"inputTokens":10}}}).
// An exception is keyed by its type ({"throttlingException":{"message":..}}),
// as the union lists it. An event that could not be decoded has no line.
func converseEventLine(ev streamEvent) []byte {
	name := strings.TrimPrefix(ev.name, "exception:")
	if name == "" || !json.Valid(ev.data) {
		return nil
	}
	line, err := json.Marshal(map[string]json.RawMessage{name: ev.data})
	if err != nil {
		return nil
	}
	out := append([]byte("data: "), line...)
	return append(out, '\n', '\n')
}
