package agenttracing

import (
	"context"
	"errors"
	"time"
)

// The read API (spec §8, §6.1). The API layer parses and bounds every
// request; the Enterprise implementation runs the queries and returns
// response bodies built from the agent-trace JSON v1 objects
// (pkg/agenttrace/traceschema/agent-trace-v1.json), so this package does not
// need the v1 Go types. Every query is scoped by Viewer.

// Limits of the read API. A request over a limit is a 400, never clamped.
const (
	MaxSessionPageSize     = 200
	DefaultSessionPageSize = 50
	MaxSpanPageSize        = 1000
	DefaultSpanPageSize    = 1000
	MaxSearchTermLength    = 256
	MaxSearchResults       = 200
	DefaultSearchResults   = 50
)

// ErrNotFound is returned for a session, trace or span that does not exist
// or that the viewer may not see: the API answers 404 either way.
var ErrNotFound = errors.New("agent trace not found")

// BadRequestError is a query the caller got wrong (a 400).
type BadRequestError struct{ Msg string }

func (e *BadRequestError) Error() string { return e.Msg }

// BadRequest returns a *BadRequestError.
func BadRequest(msg string) error { return &BadRequestError{Msg: msg} }

// Viewer is who is asking and what they may see.
type Viewer struct {
	UserID uint
	// AllApps grants every App (administrators and holders of
	// traces:read); otherwise only AppIDs are visible.
	AllApps bool
	AppIDs  []uint
	// CanReadContent, CanExport and CanPurge are reported back in the
	// session detail's viewer block so the UI can choose between content
	// and placeholders. Content is never returned by the metadata reads.
	CanReadContent bool
	CanExport      bool
	CanPurge       bool
}

// SessionQuery is the sessions list filter (spec §6.1). Zero values do not
// filter.
type SessionQuery struct {
	From, To time.Time
	AppID    uint
	UserID   uint
	// EndUser and ExternalID are client-supplied and may be personal data
	// (an email): they only ever arrive in a POST body, never in a URL.
	EndUser    string
	ExternalID string
	SessionID  string
	Agent      string
	Model      string
	Tool       string
	Status     string // active | closed | error
	Source     string // declared | inferred
	// WeakLinks keeps sessions whose weakest link is weak.
	WeakLinks        bool
	HasErrors        *bool
	HasGatewayEvents *bool
	CostMinMicros    *int64
	CostMaxMicros    *int64
	DurationMinMs    *int64
	DurationMaxMs    *int64
	// Sort is one of SessionSorts; Desc reverses it.
	Sort   string
	Desc   bool
	Cursor string
	Limit  int
}

// SessionSorts are the sortable list columns.
var SessionSorts = []string{"started_at", "cost", "duration", "tokens", "turns", "llm_calls", "tool_calls", "errors"}

// SpanQuery pages a session's (or a trace's) span skeleton, keyset-ordered
// by (start_time, span_id).
type SpanQuery struct {
	Kind    string
	Status  string
	AgentID string
	From    time.Time
	To      time.Time
	Cursor  string
	Limit   int
	// Previews asks for the inline content previews; whether they are
	// served is reported per page (see the spans response).
	Previews bool
}

// SearchQuery is the in-session find. The term arrives in a POST body.
type SearchQuery struct {
	Term  string
	Limit int
}

// Querier is the read side of the service. The Community stub answers every
// method with ErrEnterpriseFeature, and a gated Enterprise build with
// ErrDisabled. The results are response bodies, marshalled as they are.
type Querier interface {
	ListSessions(ctx context.Context, v Viewer, q SessionQuery) (any, error)
	GetSession(ctx context.Context, v Viewer, sessionID string) (any, error)
	ListSessionSpans(ctx context.Context, v Viewer, sessionID string, q SpanQuery) (any, error)
	GetTrace(ctx context.Context, v Viewer, traceID []byte, q SpanQuery) (any, error)
	GetSpan(ctx context.Context, v Viewer, spanID []byte) (any, error)
	SearchSession(ctx context.Context, v Viewer, sessionID string, q SearchQuery) (any, error)
}

// Unavailable is the inert service for a host that wired none (handler
// tests, embedders that skip it): it answers like the gated service, and
// unlike NewService it leaves the process flag alone.
func Unavailable() Service {
	if enterpriseFactory == nil {
		return communityService{err: ErrEnterpriseFeature}
	}
	return communityService{err: ErrDisabled, enterprise: true}
}

func (s communityService) ListSessions(context.Context, Viewer, SessionQuery) (any, error) {
	return nil, s.err
}
func (s communityService) GetSession(context.Context, Viewer, string) (any, error) { return nil, s.err }
func (s communityService) ListSessionSpans(context.Context, Viewer, string, SpanQuery) (any, error) {
	return nil, s.err
}
func (s communityService) GetTrace(context.Context, Viewer, []byte, SpanQuery) (any, error) {
	return nil, s.err
}
func (s communityService) GetSpan(context.Context, Viewer, []byte) (any, error) { return nil, s.err }
func (s communityService) SearchSession(context.Context, Viewer, string, SearchQuery) (any, error) {
	return nil, s.err
}
