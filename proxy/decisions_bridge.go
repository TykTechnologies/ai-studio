package proxy

// Decision-model bridge: /ai/{routeId}/v1/decisions (and the unified ingress
// {base}/decisions, rewritten to it) accepts OpenAI Decisions requests for any
// LLM whose vendor serves decision models, and answers in the OpenAI format.
//
// Like the chat bridge it is a pure bridge: auth, request filters, plugins,
// budget and analytics all run on the loopback /llm/call/{slug} hop, which
// speaks the vendor's native wire format. OpenAI LLMs are called with the body
// as sent; System One LLMs (TypeSafe Jev and wire-compatible self-hosts) get a
// translated request, and their answers are translated back.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/decisions"
	"github.com/TykTechnologies/midsommar/v2/switches"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// maxDecisionResponseBytes caps a buffered decision response. Answers are a
// few hundred bytes per question; 255-option distributions stay well below.
const maxDecisionResponseBytes = 8 << 20

// statusSiteOverloaded is System One's "temporarily overloaded" status.
const statusSiteOverloaded = 529

// errDecisionsUnsupported marks a rung whose vendor serves no decision
// models; the waterfall skips it like a rung whose driver cannot be built.
var errDecisionsUnsupported = errors.New("LLM does not serve decision models")

// CreateDecisionHandler serves POST /ai/{routeId}/v1/decisions.
func (p *Proxy) CreateDecisionHandler(w http.ResponseWriter, r *http.Request) {
	routeID := mux.Vars(r)["routeId"]

	conf, ok := p.GetLLM(routeID)
	if !ok {
		if _, isRouter := p.lookupRouter(routeID); isRouter {
			respondWithOAIError(w, http.StatusBadRequest,
				fmt.Sprintf("'%s' is a router; routers do not serve decision models, address a decision-capable LLM route instead", routeID), nil, false)
			return
		}
		respondWithOAIError(w, http.StatusNotFound, fmt.Sprintf("vendor '%s' not found or not supported by your access rights", routeID), nil, false)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUnifiedRouterBodyBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondWithOAIError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds the %d byte limit", maxUnifiedRouterBodyBytes), nil, false)
			return
		}
		respondWithOAIError(w, http.StatusBadRequest, "failed to read request body", err, false)
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		respondWithOAIError(w, http.StatusBadRequest, "invalid request body: expected an OpenAI Decisions JSON object", err, false)
		return
	}
	if stream, ok := fields["stream"]; ok && string(bytes.TrimSpace(stream)) == "true" {
		respondDecisionError(w, http.StatusBadRequest, "stream", "decision models do not stream; omit 'stream'")
		return
	}
	var req decisions.Request
	if err := json.Unmarshal(body, &req); err != nil {
		respondWithOAIError(w, http.StatusBadRequest, "invalid request body: expected an OpenAI Decisions JSON object", err, false)
		return
	}

	if !switches.SupportsDecisions(conf.Vendor) {
		respondDecisionError(w, http.StatusBadRequest, "model",
			fmt.Sprintf("LLM route '%s' (vendor %s) does not serve decision models", routeID, conf.Vendor))
		return
	}
	if !NewModelValidator(conf.AllowedModels).IsModelAllowed(req.Model) {
		respondWithOAIError(w, http.StatusForbidden, fmt.Sprintf("Model '%s' is not allowed", req.Model), nil, false)
		return
	}
	// Validate against the primary's format up front: a request the primary
	// cannot take is the caller's error, not a reason to fail over.
	primaryFormat, _ := decisions.FormatForPath(switches.DecisionEndpoint(conf.Vendor))
	if err := decisions.Validate(&req, primaryFormat); err != nil {
		respondDecisionValidation(w, err)
		return
	}
	if primaryFormat == decisions.FormatSystemOne {
		if _, err := decisions.ToSystemOne(&req); err != nil {
			respondDecisionValidation(w, err)
			return
		}
	}

	plan := decisionPlan(p.planFailover(conf, req.Model, r))
	// A fallback of another wire format may not be able to carry this
	// request (image input to a text-only System One model). Dropping it here,
	// like any other unusable rung, keeps the primary's own failure as what
	// the caller sees, rather than the fallback's translation error.
	plan = planKeepingAttempts(plan, func(a llmAttempt) bool { return decisionRungAccepts(a, &req) },
		"its wire format cannot carry this request")
	overall, cancelAll := context.WithTimeout(r.Context(), p.config.llmTimeout())
	defer cancelAll()

	var (
		out    []byte
		served llmAttempt
		last   attemptFailure
		relay  *loopbackRelay
	)
	for i, a := range plan.attempts {
		actx, cancel := context.WithTimeout(overall, plan.attemptTimeout)
		attemptRelay := &loopbackRelay{}
		var fail attemptFailure
		out, fail = p.decisionAttempt(actx, r, a, fields, &req, attemptRelay)
		cancel()
		if fail.err == nil {
			served, relay = a, attemptRelay
			break
		}
		last = fail
		if i == plan.last() || overall.Err() != nil {
			break
		}
		ok, reason := shouldFailover(last, plan.triggers)
		if !ok {
			break
		}
		p.noteFailover(r, a, plan.attempts[i+1], reason, last)
	}
	if out == nil {
		respondAttemptFailure(w, decisionClientFailure(last), "failed to get decisions")
		return
	}

	setServedHeaders(w, served)
	relay.applyTo(w.Header())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// decisionPlan drops the failover rungs whose vendor serves no decision
// models, as planFailover drops other rungs that cannot be used: a waterfall
// configured for chat must cost a decision request a fallback, not hand the
// caller a fallback's "unsupported" error in place of the primary's own.
func decisionPlan(plan failoverPlan) failoverPlan {
	return planKeeping(plan, switches.SupportsDecisions, "vendor serves no decision models")
}

// chatPlan is decisionPlan for chat: it drops decision-only rungs.
func chatPlan(plan failoverPlan) failoverPlan {
	return planKeeping(plan, switches.SupportsChat, "vendor serves decision models only")
}

// planKeeping keeps the primary and the fallback rungs whose vendor passes
// keep, renumbering the kept rungs.
func planKeeping(plan failoverPlan, keep func(models.Vendor) bool, reason string) failoverPlan {
	return planKeepingAttempts(plan, func(a llmAttempt) bool { return keep(a.conf.Vendor) }, reason)
}

// decisionRungAccepts reports whether a rung's wire format can carry req.
// The primary's format was validated before planning.
func decisionRungAccepts(a llmAttempt, req *decisions.Request) bool {
	format, ok := decisions.FormatForPath(switches.DecisionEndpoint(a.conf.Vendor))
	if !ok {
		return false
	}
	if format != decisions.FormatSystemOne {
		return true
	}
	attemptReq := *req
	attemptReq.Model = a.model
	_, err := decisions.ToSystemOne(&attemptReq)
	return err == nil
}

// planKeepingAttempts keeps the primary and the fallback rungs keep accepts,
// renumbering the kept rungs.
func planKeepingAttempts(plan failoverPlan, keep func(llmAttempt) bool, reason string) failoverPlan {
	kept := make([]llmAttempt, 1, len(plan.attempts))
	kept[0] = plan.attempts[0]
	for _, a := range plan.attempts[1:] {
		if !keep(a) {
			log.Warn().Str("target_slug", a.slug).Str("vendor", string(a.conf.Vendor)).
				Msg("failover target skipped: " + reason)
			continue
		}
		a.index = len(kept)
		kept = append(kept, a)
	}
	plan.attempts = kept
	return plan
}

// decisionAttempt runs one rung: it shapes the request for the rung's vendor,
// sends it through the loopback /llm/call/ hop and returns the OpenAI-format
// response body.
func (p *Proxy) decisionAttempt(ctx context.Context, r *http.Request, a llmAttempt,
	fields map[string]json.RawMessage, req *decisions.Request, relay *loopbackRelay) ([]byte, attemptFailure) {
	endpoint := switches.DecisionEndpoint(a.conf.Vendor)
	format, ok := decisions.FormatForPath(endpoint)
	if !ok {
		return nil, attemptFailure{err: errDecisionsUnsupported, status: http.StatusBadRequest, driverError: true}
	}

	attemptReq := *req
	attemptReq.Model = a.model
	var payload []byte
	var err error
	switch format {
	case decisions.FormatSystemOne:
		var so *decisions.SystemOneRequest
		if so, err = decisions.ToSystemOne(&attemptReq); err == nil {
			payload, err = json.Marshal(so)
		}
	default:
		// Forward the caller's body as sent (fields this gateway does not
		// know about included), with only the model set for this rung.
		forwarded := make(map[string]json.RawMessage, len(fields))
		for k, v := range fields {
			forwarded[k] = v
		}
		forwarded["model"], _ = json.Marshal(a.model)
		payload, err = json.Marshal(forwarded)
	}
	if err != nil {
		// The primary was validated up front, so this is a fallback rung of
		// another format that cannot take the request (images on a text-only
		// model): skip it.
		return nil, attemptFailure{err: fmt.Errorf("%w: %v", errDriverSetup, err), status: http.StatusBadRequest, driverError: true}
	}

	scheme := "http"
	if p.loopbackTLS() {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://127.0.0.1:%d/llm/call/%s%s", scheme, p.config.Port, a.slug, endpoint)
	if rt := timingFrom(r.Context()); rt != nil {
		ctx = withRequestTiming(ctx, rt)
	}
	ctx = withLoopbackRelay(ctx, relay)
	loopReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, attemptFailure{err: fmt.Errorf("%w: %v", errDriverSetup, err), status: http.StatusInternalServerError, driverError: true}
	}
	loopReq.Header.Set("Content-Type", "application/json")
	if id := r.Header.Get("X-Request-ID"); id != "" {
		loopReq.Header.Set("X-Request-ID", id)
	}

	client := p.newInternalRoutingClient(r.Header.Get("Authorization"), withClientLoopbackHeaders(r, p.loopbackHeaders(r.Context(), a)))
	resp, err := client.Do(loopReq)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, attemptFailure{err: err, status: http.StatusGatewayTimeout, timedOut: true}
		}
		return nil, attemptFailure{err: err, status: http.StatusBadGateway}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxDecisionResponseBytes))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, attemptFailure{err: err, status: http.StatusGatewayTimeout, timedOut: true}
		}
		return nil, attemptFailure{err: err, status: http.StatusBadGateway}
	}

	if resp.StatusCode >= http.StatusMultipleChoices {
		fail := attemptFailure{
			err:       fmt.Errorf("decision upstream returned status %d", resp.StatusCode),
			status:    resp.StatusCode,
			hasStatus: true,
		}
		if inner, ok := parseLoopbackError(resp.StatusCode, respBody); ok {
			fail.inner = inner
		} else if msg := upstreamErrorText(respBody); msg != "" {
			fail.inner = &APIError{Message: msg}
		}
		return nil, fail
	}

	if format != decisions.FormatSystemOne {
		return respBody, attemptFailure{}
	}
	var so decisions.SystemOneResponse
	if err := json.Unmarshal(respBody, &so); err != nil || so.Answers == nil {
		return nil, attemptFailure{err: fmt.Errorf("decision upstream returned an unreadable response: %v", err), status: http.StatusBadGateway, hasStatus: true}
	}
	out, err := json.Marshal(decisions.FromSystemOne(&so, &attemptReq))
	if err != nil {
		return nil, attemptFailure{err: err, status: http.StatusInternalServerError}
	}
	log.Debug().Str("llm", a.slug).Int("questions", len(req.Questions)).Msg("decision bridge translated System One response")
	return out, attemptFailure{}
}

// decisionClientFailure maps upstream statuses the OpenAI contract does not
// use onto ones its SDKs understand: System One's 422 validation failure is
// a 400, and its 529 "overloaded" a retryable 503.
func decisionClientFailure(f attemptFailure) attemptFailure {
	switch f.status {
	case http.StatusUnprocessableEntity:
		f.status = http.StatusBadRequest
		if f.inner != nil {
			inner := *f.inner
			inner.Type, inner.Code = "invalid_request_error", nil
			f.inner = &inner
		}
	case statusSiteOverloaded:
		f.status = http.StatusServiceUnavailable
		if f.inner != nil {
			inner := *f.inner
			inner.Type, inner.Code = "", nil
			f.inner = &inner
		}
	}
	if errors.Is(f.err, errDecisionsUnsupported) && f.inner == nil {
		f.inner = &APIError{Message: f.err.Error(), Type: "invalid_request_error", Code: "unsupported_model_class"}
	}
	return f
}

// upstreamErrorText recovers a readable message from an error body that is
// not an OpenAI envelope: System One answers {"detail": ...} or a plain
// string. Bodies that are neither yield "".
func upstreamErrorText(body []byte) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) == nil {
		for _, k := range []string{"detail", "message", "error"} {
			if v, ok := obj[k]; ok {
				var s string
				if json.Unmarshal(v, &s) == nil && s != "" {
					return s
				}
				var buf bytes.Buffer
				if json.Compact(&buf, v) == nil {
					return buf.String()
				}
			}
		}
		return ""
	}
	if text := bytes.TrimSpace(body); len(text) > 0 && len(text) < 512 {
		return string(text)
	}
	return ""
}

// respondDecisionValidation answers a request that failed decision validation.
func respondDecisionValidation(w http.ResponseWriter, err error) {
	var verr *decisions.ValidationError
	if errors.As(err, &verr) {
		respondDecisionError(w, http.StatusBadRequest, verr.Param, verr.Message)
		return
	}
	respondDecisionError(w, http.StatusBadRequest, "", err.Error())
}

// respondDecisionError writes an invalid_request_error naming the offending
// parameter, the way OpenAI reports request validation failures.
func respondDecisionError(w http.ResponseWriter, status int, param, message string) {
	apiErr := &APIError{Message: message, Type: "invalid_request_error"}
	if param != "" {
		apiErr.Param = &param
	}
	writeOAIError(w, status, apiErr)
}

// decisionOnlyVendorError is the chat bridge's answer for an LLM whose vendor
// serves decision models only.
func decisionOnlyVendorError(w http.ResponseWriter, routeID string, vendor models.Vendor) {
	respondDecisionError(w, http.StatusBadRequest, "model",
		fmt.Sprintf("LLM route '%s' (vendor %s) serves decision models only; send OpenAI Decisions requests to /v1/decisions", routeID, vendor))
}
