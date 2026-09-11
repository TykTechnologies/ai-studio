package proxy

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/modelmatch"
	"github.com/gosimple/slug"
	"github.com/rs/zerolog/log"
	"github.com/tmc/langchaingo/llms"
)

// LLM failover.
//
// An LLM config may carry a waterfall of (LLM, model) rungs. The outer /ai/
// translator hop is the only place that still holds a vendor-neutral request,
// so that is where the loop lives: the primary is tried, and on a qualifying
// failure each rung is tried in turn with the request re-targeted at that
// rung's LLM and model. Everything below the loop -- auth, budget, filters,
// vendor auth, analytics -- keeps running once per attempt on the inner
// /llm/call/ hop exactly as it does for a plain request.
//
// Two things need care and are handled here rather than in the loop itself:
//
//   - Access to a fallback is inherited from the primary. The inner hop checks
//     that the app was granted the LLM named in the path, which a fallback
//     usually is not. The loop marks its loopback requests with headers the
//     inner hop honours only when they carry this process's random token, and
//     only when the app really is allowed the origin LLM and the origin's
//     waterfall really lists the target. Nothing an external caller sends can
//     forge that.
//   - Each attempt writes its own ProxyLog on the inner hop. The same marker
//     lets those rows say which primary they were failing over from, so a
//     failed primary row and a successful fallback row can be read together.

// Loopback marker headers. Never forwarded to a vendor: the REST director and
// the streaming hop strip them before egress.
const (
	hdrFailoverOrigin  = "X-Tyk-Failover-Origin"  // primary LLM slug
	hdrFailoverAttempt = "X-Tyk-Failover-Attempt" // 1-based rung index
	hdrFailoverToken   = "X-Tyk-Failover-Token"   // per-process secret
)

// Client-facing response headers.
const (
	hdrFailover    = "X-Tyk-Failover"     // "true" when a fallback served
	hdrServedLLM   = "X-Tyk-Served-LLM"   // slug of the LLM that answered
	hdrServedModel = "X-Tyk-Served-Model" // model that answered
)

// llmAttempt is one rung of the waterfall as the loop sees it: the LLM config
// to call and the model to ask it for. Index 0 is the primary.
type llmAttempt struct {
	conf   *models.LLM
	slug   string
	model  string
	index  int
	origin *models.LLM // primary; nil on the primary itself
}

// failoverPlan is the ordered attempt list plus the resolved trigger rules.
type failoverPlan struct {
	attempts       []llmAttempt
	triggers       models.ResolvedFailoverTriggers
	attemptTimeout time.Duration
}

// last reports the index of the final attempt.
func (fp failoverPlan) last() int { return len(fp.attempts) - 1 }

// newFailoverToken makes the per-process secret the loopback marker carries.
func newFailoverToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// rand.Read failing means the process has bigger problems; a fixed
		// token here would silently disable the spoof guard, so refuse instead.
		panic("failover: cannot generate token: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// planFailover builds the attempt list for a request to primary asking for
// requestedModel. Rungs that cannot be used right now are dropped with a
// warning rather than failing the request: a target that was deactivated, or
// whose allowed models changed after the primary was saved, should cost the
// caller one fewer fallback, not a hard error.
func (p *Proxy) planFailover(primary *models.LLM, requestedModel string, r *http.Request) failoverPlan {
	plan := failoverPlan{
		attempts: []llmAttempt{{conf: primary, slug: slug.Make(primary.Name), model: requestedModel}},
		triggers: primary.Failover.EffectiveTriggers(),
	}
	plan.attemptTimeout = p.config.llmTimeout()
	if plan.triggers.AttemptTimeoutSecond > 0 {
		plan.attemptTimeout = time.Duration(plan.triggers.AttemptTimeoutSecond) * time.Second
	}
	if !primary.Failover.Enabled() {
		return plan
	}

	app, _ := r.Context().Value("app").(*models.App)
	skip := func(i int, t models.LLMFailoverTarget, reason string) {
		log.Warn().
			Uint("primary_llm_id", primary.ID).
			Str("primary_slug", plan.attempts[0].slug).
			Uint("target_llm_id", t.LLMID).
			Int("target_index", i+1).
			Str("reason", reason).
			Msg("failover target skipped")
	}

	for i, t := range primary.Failover.Targets {
		conf, ok := p.GetLLMByID(t.LLMID)
		if !ok {
			skip(i, t, "target LLM not loaded (deleted or inactive)")
			continue
		}
		if conf.ID == primary.ID {
			skip(i, t, "target is the primary")
			continue
		}
		if !modelmatch.Allowed(conf.AllowedModels, t.Model) {
			skip(i, t, "model is no longer in the target's allowed models")
			continue
		}
		if app != nil && p.budgetService != nil {
			if _, _, err := p.budgetService.CheckBudget(app, conf); err != nil {
				// Advisory only; the inner hop's check is the one that counts.
				skip(i, t, "budget exceeded: "+err.Error())
				continue
			}
		}
		plan.attempts = append(plan.attempts, llmAttempt{
			conf:   conf,
			slug:   slug.Make(conf.Name),
			model:  t.Model,
			index:  len(plan.attempts),
			origin: primary,
		})
	}
	return plan
}

// attemptFailure is a failed attempt, already normalised: the status to report
// (502 when the error carried none) and whether it carried one at all. The
// distinction matters because a connection failure with no status is decided
// by the timeout / connection-error triggers, not the status list.
type attemptFailure struct {
	err       error
	status    int
	hasStatus bool
	// timedOut is set when the attempt's own deadline expired. Drivers
	// sanitise that into a plain string error (the OpenAI client returns
	// errors.New("request timeout: ...")), so the loop records it from the
	// attempt context rather than trusting the error's shape.
	timedOut bool
	// driverError is set when the rung's vendor driver could not even be
	// built: a problem with that rung's config, never with the request, so
	// the next rung is always worth trying.
	driverError bool
}

// errDriverSetup wraps a driver construction failure so classification can
// tell it apart from a failed call.
var errDriverSetup = errors.New("driver setup failed")

// classifyDriverError normalises a driver error. attemptCtx is the attempt's
// context as it stood when the driver returned, before it is cancelled.
func classifyDriverError(err error, attemptCtx context.Context) attemptFailure {
	if errors.Is(err, errDriverSetup) {
		return attemptFailure{err: err, status: http.StatusInternalServerError, driverError: true}
	}
	if attemptCtx != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return attemptFailure{err: err, status: http.StatusGatewayTimeout, timedOut: true}
	}
	status, ok := upstreamStatusFromError(err, http.StatusBadGateway)
	return attemptFailure{err: err, status: status, hasStatus: ok}
}

// classifyBedrockError is classifyDriverError for the AWS SDK, whose service
// errors expose the HTTP status directly.
func classifyBedrockError(err error, attemptCtx context.Context) attemptFailure {
	if errors.Is(err, errDriverSetup) {
		return attemptFailure{err: err, status: http.StatusInternalServerError, driverError: true}
	}
	if attemptCtx != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return attemptFailure{err: err, status: http.StatusGatewayTimeout, timedOut: true}
	}
	if status := bedrockErrorStatus(err, 0); status != 0 {
		return attemptFailure{err: err, status: status, hasStatus: true}
	}
	return attemptFailure{err: err, status: http.StatusBadGateway}
}

// requestForAttempt is r with the failover marker on its context for a
// fallback rung. Bedrock rungs run inline rather than through the loopback
// hop, so their analytics read the marker from the context instead of the
// loopback headers.
func requestForAttempt(r *http.Request, a llmAttempt) *http.Request {
	if a.origin == nil {
		return r
	}
	return r.WithContext(withFailoverMarker(r.Context(), failoverMarker{FromLLMID: a.origin.ID, Attempt: a.index}))
}

// shouldFailover decides whether the next rung is worth trying after f, and
// names the reason for the log line and the metric label. Reasons are a small
// fixed set so the metric's cardinality stays bounded.
func shouldFailover(f attemptFailure, trig models.ResolvedFailoverTriggers) (bool, string) {
	err := f.err
	if err == nil {
		return false, ""
	}
	if f.driverError {
		return true, "driver_error"
	}
	if f.timedOut {
		return trig.OnTimeout, "timeout"
	}
	if errors.Is(err, context.Canceled) {
		// The caller went away; there is nobody to fail over for.
		return false, ""
	}
	if f.hasStatus {
		// An explicit vendor status wins over the error's shape. 4xx other
		// than 408/429 are the caller's or the config's problem and every
		// rung would repeat them.
		if !models.FailoverStatusCodeAllowed(f.status) {
			return false, ""
		}
		if trig.StatusCodes[f.status] {
			return true, "status_" + strconv.Itoa(f.status)
		}
		return false, ""
	}
	// *url.Error also implements net.Error, so the timeout test must come
	// first or a timed-out dial reads as a connection error.
	if isTimeoutErr(err) {
		return trig.OnTimeout, "timeout"
	}
	if isConnectionErr(err) {
		return trig.OnConnectionError, "connection_error"
	}
	return false, ""
}

func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var llmErr *llms.Error
	return errors.As(err, &llmErr) && llmErr.Code == llms.ErrCodeTimeout
}

func isConnectionErr(err error) bool {
	var urlErr *url.Error
	var opErr *net.OpError
	return errors.As(err, &urlErr) ||
		errors.As(err, &opErr) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET)
}

// failoverHeaders are added to the loopback request for a fallback rung so the
// inner hop can grant inherited access and mark its analytics. Nil for the
// primary, which needs neither.
func (p *Proxy) failoverHeaders(a llmAttempt) http.Header {
	if a.origin == nil {
		return nil
	}
	h := http.Header{}
	h.Set(hdrFailoverOrigin, slug.Make(a.origin.Name))
	h.Set(hdrFailoverAttempt, strconv.Itoa(a.index))
	h.Set(hdrFailoverToken, p.failoverToken)
	return h
}

// failoverMarker is what the inner hop learns from a trusted marker.
type failoverMarker struct {
	FromLLMID uint
	Attempt   int
}

type failoverMarkerKey struct{}

func withFailoverMarker(ctx context.Context, m failoverMarker) context.Context {
	return context.WithValue(ctx, failoverMarkerKey{}, m)
}

func failoverMarkerFromContext(ctx context.Context) (failoverMarker, bool) {
	m, ok := ctx.Value(failoverMarkerKey{}).(failoverMarker)
	return m, ok
}

// tokenMatches is the spoof guard: the marker is trusted only with this
// process's token.
func (p *Proxy) tokenMatches(r *http.Request) bool {
	got := r.Header.Get(hdrFailoverToken)
	return got != "" && p.failoverToken != "" &&
		subtle.ConstantTimeCompare([]byte(got), []byte(p.failoverToken)) == 1
}

// parseFailoverMarker reads a trusted marker off the inner-hop request. A
// marker with a bad token, an unknown origin or a bad attempt number is
// ignored, never an error: it can only come from outside, and outside callers
// get exactly the behaviour they would have had without it.
func (p *Proxy) parseFailoverMarker(r *http.Request) (failoverMarker, bool) {
	if !p.tokenMatches(r) {
		return failoverMarker{}, false
	}
	origin, ok := p.GetLLM(r.Header.Get(hdrFailoverOrigin))
	if !ok {
		return failoverMarker{}, false
	}
	attempt, err := strconv.Atoi(r.Header.Get(hdrFailoverAttempt))
	if err != nil || attempt < 1 {
		return failoverMarker{}, false
	}
	return failoverMarker{FromLLMID: origin.ID, Attempt: attempt}, true
}

// failoverGrantsAccess is the inherited-access rule the inner hop applies
// after the app's own LLM list has said no: the request must carry a valid
// marker, the app must be allowed the origin LLM, and the origin's waterfall
// must list the target. All three are things only the proxy itself can
// arrange.
func (p *Proxy) failoverGrantsAccess(r *http.Request, app *models.App, target *models.LLM) bool {
	if app == nil || target == nil || !p.tokenMatches(r) {
		return false
	}
	origin, ok := p.GetLLM(r.Header.Get(hdrFailoverOrigin))
	if !ok || origin.ID == target.ID {
		return false
	}
	allowedOrigin := false
	for _, l := range app.LLMs {
		if l.ID == origin.ID {
			allowedOrigin = true
			break
		}
	}
	if !allowedOrigin {
		return false
	}
	for _, t := range origin.Failover.Targets {
		if t.LLMID == target.ID {
			return true
		}
	}
	return false
}

// stripFailoverHeaders removes the loopback marker before a request leaves
// for the vendor.
func stripFailoverHeaders(h http.Header) {
	h.Del(hdrFailoverOrigin)
	h.Del(hdrFailoverAttempt)
	h.Del(hdrFailoverToken)
}

// applyFailoverMarker stamps a ProxyLog with the marker from ctx, if any.
func applyFailoverMarker(l *models.ProxyLog, ctx context.Context) {
	m, ok := failoverMarkerFromContext(ctx)
	if !ok || m.Attempt < 1 {
		return
	}
	from := m.FromLLMID
	l.FailoverFromLLMID = &from
	l.FailoverAttempt = m.Attempt
}

// setServedHeaders tells the client which LLM and model actually answered.
// Called before anything is written so the streaming path can still set them.
func setServedHeaders(w http.ResponseWriter, a llmAttempt) {
	h := w.Header()
	h.Set(hdrServedLLM, a.slug)
	h.Set(hdrServedModel, a.model)
	if a.index > 0 {
		h.Set(hdrFailover, "true")
	} else {
		h.Del(hdrFailover)
	}
}

// clearServedHeaders removes the served headers when the request failed after
// all, so an error response does not claim an LLM served it.
func clearServedHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Del(hdrServedLLM)
	h.Del(hdrServedModel)
	h.Del(hdrFailover)
}

// GetLLMByID looks a loaded LLM up by its database id. Waterfall rungs are
// stored by id, so this is the lookup the loop needs; the slug map is what
// everything else uses.
func (p *Proxy) GetLLMByID(id uint) (*models.LLM, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	llm, ok := p.llmsByID[id]
	return llm, ok
}
