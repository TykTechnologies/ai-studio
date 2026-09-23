// Package semanticrouting holds the shared shape of a Semantic Router: the
// configuration an administrator writes, what the classifier is asked, and
// what it answers. The classifier itself (the Engine) is an Enterprise
// feature, registered at start-up by the Enterprise build; this package only
// defines the contract, so the hub, the edge and the Engine agree on it.
//
// A Semantic Router picks one of its named routes for each request from what
// the prompt says, in stages (the first one that decides wins):
//
//	explicit   the caller named a route ("{router}/{route}"), when allowed
//	affinity   a session already pinned to a route (optional)
//	keyword    a route's literal or regex keyword matched the input
//	embedding  the input is close enough to one of a route's examples
//	judge      an LLM picked a route from their descriptions (optional)
//	default    nothing decided; the router's default route serves
//
// The Engine never fails a request: an error in a stage falls through to the
// next one, and the default route always answers.
package semanticrouting

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Mode decides whether a decision is acted on.
type Mode string

const (
	// ModeEnforce routes each request to the route the classifier picks.
	ModeEnforce Mode = "enforce"
	// ModeShadow classifies and records the pick, but always serves the
	// default route: a way to try a router on live traffic.
	ModeShadow Mode = "shadow"
)

// InputScope is which part of the conversation is classified.
type InputScope string

const (
	// ScopeLastUser classifies the last user message (the default): what
	// the caller is asking now, without earlier turns pulling it elsewhere.
	ScopeLastUser InputScope = "last_user"
	// ScopeAllUser classifies every user message, joined.
	ScopeAllUser InputScope = "all_user"
)

// JudgeWhen is when the LLM judge runs.
type JudgeWhen string

const (
	// JudgeAlways makes the judge the classifier: it decides whenever
	// keywords did not, and the routes' examples are not consulted.
	JudgeAlways JudgeWhen = "always"
	// JudgeLowConfidence (the default) runs the judge only when neither
	// keywords nor examples decided.
	JudgeLowConfidence JudgeWhen = "low_confidence"
)

// TargetType is what a route sends a request to.
type TargetType string

const (
	// TargetLLM sends the request to an LLM with a model.
	TargetLLM TargetType = "llm"
	// TargetModelRouter hands the request to a Model Router with a model
	// (typically an alias the Model Router maps and balances).
	TargetModelRouter TargetType = "model_router"
)

// ReservedAutoModel is the model a caller sends ("{router}/auto") to have the
// router classify the request. It cannot be a route name.
const ReservedAutoModel = "auto"

// Defaults applied when a setting is left at its zero value.
const (
	DefaultThreshold          = 0.75
	DefaultMaxInputChars      = 4000
	DefaultEmbeddingTimeoutMs = 1500
	DefaultJudgeTimeoutMs     = 3000
	DefaultAffinityTTLSeconds = 1800
	DefaultAffinityHeader     = "X-Tyk-Session-Id"
)

// ModelRef names an LLM (by id) and the model to ask it for.
type ModelRef struct {
	LLMID     uint   `json:"llm_id"`
	Model     string `json:"model"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// Keyword is one keyword of a route. A literal matches case-insensitively as
// a substring; a regex is Go RE2 syntax, matched as written.
type Keyword struct {
	Pattern string `json:"pattern"`
	Regex   bool   `json:"regex,omitempty"`
}

// Target is where a route sends a request.
type Target struct {
	Type          TargetType `json:"type"`
	LLMID         uint       `json:"llm_id,omitempty"`
	ModelRouterID uint       `json:"model_router_id,omitempty"`
	Model         string     `json:"model"`
}

// Route is one named route of a router.
type Route struct {
	// Name is the route's identifier ("complex", "code"): what the judge
	// answers with and what "{router}/{route}" names.
	Name string `json:"name"`
	// Description says what belongs on this route. The judge reads it, and
	// the portal shows it.
	Description string `json:"description,omitempty"`
	// Priority breaks ties between routes (higher first).
	Priority int `json:"priority,omitempty"`
	// Keywords decide the route outright when one matches.
	Keywords []Keyword `json:"keywords,omitempty"`
	// Utterances are example requests. The input is compared with each and
	// the route scores its best match (cosine similarity).
	Utterances []string `json:"utterances,omitempty"`
	// Threshold is the similarity (0..1] the best example must reach for the
	// route to be picked by the embedding stage. 0 means DefaultThreshold.
	Threshold float64 `json:"threshold,omitempty"`
	Target    Target  `json:"target"`
}

// JudgeSettings configures the LLM judge.
type JudgeSettings struct {
	Enabled  bool      `json:"enabled"`
	ModelRef ModelRef  `json:"model_ref"`
	When     JudgeWhen `json:"when,omitempty"`
}

// AffinitySettings pins a session to the route its first request got, so a
// conversation does not switch models mid-thread (which also keeps provider
// prompt caches warm). Sessions are named by a request header, per App, and
// remembered on each gateway node.
type AffinitySettings struct {
	Enabled    bool   `json:"enabled"`
	Header     string `json:"header,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// Settings is everything about a router except its routes.
type Settings struct {
	Mode               Mode             `json:"mode,omitempty"`
	AllowExplicitRoute bool             `json:"allow_explicit_route"`
	InputScope         InputScope       `json:"input_scope,omitempty"`
	MaxInputChars      int              `json:"max_input_chars,omitempty"`
	Embedding          *ModelRef        `json:"embedding,omitempty"`
	Judge              JudgeSettings    `json:"judge"`
	Affinity           AffinitySettings `json:"affinity"`
	DefaultRoute       string           `json:"default_route"`
}

// Config is a router as the Engine compiles it.
type Config struct {
	RouterID uint     `json:"router_id"`
	Slug     string   `json:"slug"`
	Settings Settings `json:"settings"`
	Routes   []Route  `json:"routes"`
}

// Route returns the named route.
func (c *Config) Route(name string) (Route, bool) {
	for _, r := range c.Routes {
		if r.Name == name {
			return r, true
		}
	}
	return Route{}, false
}

// EffectiveMode is the mode, enforce when unset.
func (s Settings) EffectiveMode() Mode {
	if s.Mode == ModeShadow {
		return ModeShadow
	}
	return ModeEnforce
}

// Message is one message of the conversation being classified.
type Message struct {
	Role    string
	Content string
}

// Request is what the Engine is asked to classify.
type Request struct {
	// Model is what followed the router's slug in the caller's model string:
	// ReservedAutoModel, or a route name.
	Model    string
	Messages []Message
	// AffinityKey names the caller's session (App and session header), empty
	// when there is none.
	AffinityKey string
}

// Decision reasons, the stage that decided.
const (
	ReasonExplicit        = "explicit"
	ReasonAffinity        = "affinity"
	ReasonKeyword         = "keyword"
	ReasonEmbedding       = "embedding"
	ReasonJudge           = "judge"
	ReasonDefault         = "default"
	ReasonClassifierError = "classifier_error"
)

// StageResult is one stage of a classification, for the test panel.
type StageResult struct {
	Stage string `json:"stage"`
	// Route is the route the stage picked, empty when it did not decide.
	Route string `json:"route,omitempty"`
	// Scores are the embedding stage's best similarity per route.
	Scores    map[string]float64 `json:"scores,omitempty"`
	Detail    string             `json:"detail,omitempty"`
	Error     string             `json:"error,omitempty"`
	LatencyMs int64              `json:"latency_ms"`
	Skipped   bool               `json:"skipped,omitempty"`
}

// Decision is the Engine's answer.
type Decision struct {
	// Route is the route that serves the request: the classifier's pick, or
	// the default route in shadow mode.
	Route string `json:"route"`
	// Reason is the stage that decided (Reason* constants).
	Reason string `json:"reason"`
	// Score is the similarity that decided an embedding match, 0 otherwise.
	Score float64 `json:"score,omitempty"`
	// ShadowRoute is, in shadow mode, the route the classifier picked.
	ShadowRoute string `json:"shadow_route,omitempty"`
	// Input is the text that was classified (truncated), for the test panel.
	Input   string        `json:"input,omitempty"`
	Trace   []StageResult `json:"trace"`
	Latency time.Duration `json:"-"`
	// LatencyMs mirrors Latency for JSON.
	LatencyMs int64 `json:"latency_ms"`
}

// Embedder turns texts into vectors with an LLM's embedding model.
type Embedder interface {
	Embed(ctx context.Context, ref ModelRef, texts []string) ([][]float32, error)
}

// Completer asks an LLM for a completion (the judge).
type Completer interface {
	Complete(ctx context.Context, ref ModelRef, system, user string) (string, error)
}

// Router is a compiled router, safe for concurrent use.
type Router interface {
	Config() Config
	Classify(ctx context.Context, req Request) Decision
}

// Engine compiles routers.
type Engine interface {
	Compile(cfg Config) (Router, error)
}

// Deps is what the host gives the Engine: how to reach LLMs.
type Deps struct {
	Embedder  Embedder
	Completer Completer
}

// ErrUnavailable is returned where no Engine is registered (Community
// Edition): Semantic Routers are an Enterprise feature.
var ErrUnavailable = errors.New("semantic routing is an Enterprise feature")

var (
	engineMu      sync.RWMutex
	engineFactory func(Deps) Engine
)

// RegisterEngine installs the Engine implementation. Called from the init()
// of the Enterprise package.
func RegisterEngine(f func(Deps) Engine) {
	engineMu.Lock()
	defer engineMu.Unlock()
	engineFactory = f
}

// Available reports whether an Engine is registered in this build.
func Available() bool {
	engineMu.RLock()
	defer engineMu.RUnlock()
	return engineFactory != nil
}

// NewEngine builds the registered Engine with the host's dependencies.
func NewEngine(d Deps) (Engine, error) {
	engineMu.RLock()
	f := engineFactory
	engineMu.RUnlock()
	if f == nil {
		return nil, ErrUnavailable
	}
	return f(d), nil
}
