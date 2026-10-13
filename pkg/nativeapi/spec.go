package nativeapi

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Provider is implemented by an LLMVendorProvider that exposes native API
// extensions. The proxy type-asserts it on the vendor provider; vendors that
// do not implement it keep the legacy native chain unchanged.
type Provider interface {
	NativeAPI() Spec
}

// Spec is a vendor's complete set of native extensions.
type Spec struct {
	Resources  []ResourceType
	Operations []Operation

	// ChainHooks are hooks on the existing synchronous chain: calls that stay
	// on the legacy native pass-through (model validation, filters, usage
	// analysis) but hand out or reference vendor resource IDs, such as
	// OpenAI's POST /v1/responses and its previous_response_id.
	ChainHooks *ChainHooks
}

// ChainHooks are a vendor's hooks on the synchronous chain.
type ChainHooks struct {
	Hooks []ChainHook
}

// ChainHook applies to synchronous-chain requests with exactly Method and
// the canonical Path (no parameters). The proxy verifies and unwraps
// RequestRefs before the request filters run (any failure is the same 404 as
// for a native operation), and wraps ResponseIDs in a successful JSON
// response and in the resource object of every server-sent event.
//
// When the created object is a tracked async resource (ResponseIDs[0]
// names a ResourceType with Async, and Async.Tracked reports the object),
// the proxy records it in the native resource ledger so the reconciler
// accounts it; the vendor's synchronous usage analysis must then record no
// usage for it, or it is counted twice.
type ChainHook struct {
	Name   string
	Method string
	Path   string
	// RequestRefs are IDs in the request body to verify and unwrap.
	RequestRefs []BodyRef
	// ResponseIDs are IDs in the resource object to wrap. The resource
	// object is the response body, or EventObject inside an event's data.
	ResponseIDs []BodyRef
	// EventObject is the dotted path of the resource object inside a
	// server-sent event's JSON data ("response"); events without it pass
	// through unchanged. Empty: streamed responses are not rewritten.
	EventObject string
	// DropHeaders are client headers removed before the request leaves the
	// gateway, so it reaches the vendor account the native operations and
	// the reconciler use (OpenAI-Project would create a response elsewhere).
	DropHeaders []string
	// FilterFormat is the format the request filters read the body as (a
	// key of the proxy's message extractor and reconstructor registries,
	// such as "openai_responses"); empty means the vendor's own.
	FilterFormat string
	// CheckBody, when set, vets the request body before the model checks
	// and filters see it (CheckJSONKeys refuses duplicate keys); an error is
	// a 400 naming it.
	CheckBody func(body []byte) error
	// ErrorShape renders a vendor-shaped error body.
	ErrorShape func(status int, typ, msg string) []byte
}

// Declares reports whether a chain hook declares exactly path, for any
// method.
func (h *ChainHooks) Declares(path string) bool {
	if h == nil {
		return false
	}
	for i := range h.Hooks {
		if h.Hooks[i].Path == path {
			return true
		}
	}
	return false
}

// Hook returns the chain hook declared for method and the canonical path.
func (h *ChainHooks) Hook(method, path string) (*ChainHook, bool) {
	if h == nil {
		return nil, false
	}
	for i := range h.Hooks {
		if h.Hooks[i].Method == method && h.Hooks[i].Path == path {
			return &h.Hooks[i], true
		}
	}
	return nil, false
}

// Resource returns the declared resource type with the given name.
func (s Spec) Resource(name string) (ResourceType, bool) {
	for _, rt := range s.Resources {
		if rt.Name == name {
			return rt, true
		}
	}
	return ResourceType{}, false
}

// DefaultIDCharset is used for a ResourceType without an IDCharset.
var DefaultIDCharset = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,256}$`)

// ResourceType is a vendor-side object the gateway hands out IDs for.
type ResourceType struct {
	// Name is namespaced by vendor, e.g. "anthropic.message_batch". It is
	// bound into the signed ID.
	Name string
	// IDCharset is an anchored (^...$) regexp a raw vendor ID must match.
	// It is always applied to the whole ID (as ^(?:...)$), so a top-level
	// alternation such as ^a|b$ cannot match a substring. Nil means
	// DefaultIDCharset.
	IDCharset *regexp.Regexp
	// Async, when non-nil, marks a long-running resource: it is ledgered,
	// reconciled and accounted exactly once.
	Async *AsyncSpec
}

// fullMatchers caches the whole-string form of each IDCharset.
var fullMatchers sync.Map // *regexp.Regexp -> *regexp.Regexp

func fullMatch(re *regexp.Regexp) *regexp.Regexp {
	if v, ok := fullMatchers.Load(re); ok {
		return v.(*regexp.Regexp)
	}
	full := regexp.MustCompile(`^(?:` + re.String() + `)$`)
	v, _ := fullMatchers.LoadOrStore(re, full)
	return v.(*regexp.Regexp)
}

func (rt ResourceType) charset() *regexp.Regexp {
	if rt.IDCharset != nil {
		return fullMatch(rt.IDCharset)
	}
	return DefaultIDCharset
}

// ValidVendorID reports whether id is an acceptable raw vendor ID for rt: it
// matches the whole charset and contains at least one letter or digit, so
// "." and ".." can never become path segments upstream.
func (rt ResourceType) ValidVendorID(id string) bool {
	return hasAlnum(id) && rt.charset().MatchString(id)
}

func hasAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}

// UpstreamReq is a request the proxy builds against the vendor's upstream
// endpoint. Path is a canonical vendor path; the proxy joins it to the LLM's
// configured endpoint and never follows a vendor-supplied URL.
type UpstreamReq struct {
	Method string
	Path   string
	Query  url.Values
	// Header holds default headers the vendor needs (an API version) when
	// the gateway calls it without a client request (the reconciler); a
	// client request's allow-listed headers take precedence.
	Header http.Header
}

// ResourceState is the coarse lifecycle state of an async resource.
type ResourceState string

const (
	StateInProgress ResourceState = "in_progress"
	StateCanceling  ResourceState = "canceling"
	StateEnded      ResourceState = "ended"
)

// StateInfo is what AsyncSpec.State extracts from a vendor object.
type StateInfo struct {
	State           ResourceState
	EndedAt         *time.Time
	ResultsExpireAt *time.Time
}

// Usage is the billable usage of one result line (or an aggregate of lines).
// It implements models.ITokenResponse so the proxy can price it through the
// normal chat-record path, and exposes the service tier and price multiplier
// for price-adjusted accounting.
type Usage struct {
	Model                    string
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	// Tier is the vendor service tier the usage was billed at, e.g. "batch".
	Tier string
	// Multiplier scales the list price (0.5 for batch). Zero means 1.
	Multiplier float64
	// Billed is false for lines the vendor does not charge for (errored,
	// canceled, expired).
	Billed bool
}

var _ models.IPriceAdjustedResponse = Usage{}

func (u Usage) GetPromptTokens() int           { return u.InputTokens }
func (u Usage) GetResponseTokens() int         { return u.OutputTokens }
func (u Usage) GetChoiceCount() int            { return 1 }
func (u Usage) GetToolCount() int              { return 0 }
func (u Usage) GetModel() string               { return u.Model }
func (u Usage) GetCacheWritePromptTokens() int { return u.CacheCreationInputTokens }
func (u Usage) GetCacheReadPromptTokens() int  { return u.CacheReadInputTokens }
func (u Usage) ServiceTier() string            { return u.Tier }
func (u Usage) PriceMultiplier() float64 {
	if u.Multiplier == 0 {
		return 1
	}
	return u.Multiplier
}

// AsyncSpec describes how the reconciler drives a long-running resource.
type AsyncSpec struct {
	// Poll returns the request that fetches the current vendor object.
	Poll func(vendorID string) UpstreamReq
	// State extracts the lifecycle state from a vendor object.
	State func(obj []byte) (StateInfo, error)
	// Results returns the request that downloads the results, and false if
	// the object has no results (yet).
	Results func(vendorID string, obj []byte) (UpstreamReq, bool)
	// LineUsage extracts usage from one results line. ok=false means the line
	// carries no usage.
	LineUsage func(line []byte) (u Usage, ok bool, err error)
	// Cancel returns the request that cancels the resource upstream; nil if
	// it cannot be cancelled. The proxy cancels a resource it created but
	// could not record in its ledger, so it never runs unaccounted.
	Cancel func(vendorID string) UpstreamReq
	// ObjectUsage, when set, reads the usage of an ended resource from the
	// polled object itself (an OpenAI background response) instead of a
	// results download; Results and LineUsage are then not used. ok=false
	// means the object carries no billable usage.
	ObjectUsage func(obj []byte) (u Usage, ok bool, err error)
	// Tracked, when set, reports whether an object of the type is a
	// long-running resource the gateway ledgers and accounts (an OpenAI
	// response created with background: true); other objects of the type
	// are accounted synchronously and never ledgered. Nil: every object is.
	Tracked func(obj []byte) bool
}

// IsTracked reports whether obj is ledgered and accounted asynchronously.
func (a *AsyncSpec) IsTracked(obj []byte) bool {
	return a != nil && (a.Tracked == nil || a.Tracked(obj))
}

// LedgerEffect is what an operation does to the native resource ledger.
type LedgerEffect int

const (
	LedgerNone LedgerEffect = iota
	LedgerCreate
	LedgerRead
	LedgerList
	LedgerMutate
	LedgerDelete
)

func (e LedgerEffect) String() string {
	switch e {
	case LedgerNone:
		return "none"
	case LedgerCreate:
		return "create"
	case LedgerRead:
		return "read"
	case LedgerList:
		return "list"
	case LedgerMutate:
		return "mutate"
	case LedgerDelete:
		return "delete"
	}
	return fmt.Sprintf("LedgerEffect(%d)", int(e))
}

// Item is one admission unit of a request body: one batch request, one JSONL
// line of an uploaded file, or the whole body for a single-body operation.
type Item struct {
	// Key identifies the item to the client (e.g. custom_id) in errors and
	// filter script context.
	Key string
	// Format names the message format the proxy's filter extractor reads the
	// item Body as (e.g. "anthropic").
	Format string
	// Model is the model the item asks for; checked against the LLM's
	// allow-list. Empty: the item invokes no model (items stored in an
	// OpenAI conversation) and only the filters run on it. A Split for an
	// item that does invoke a model must refuse one without it.
	Model string
	// Body is the item's request body as the filters see it.
	Body []byte
}

// AdmitSpec splits a request body into items that are admitted (model
// allow-list, request filters) one by one, and joins them back after filters
// may have rewritten some of them.
type AdmitSpec struct {
	// Multipart marks a multipart/form-data body (an OpenAI file upload).
	// The proxy requires the Content-Type boundary to open the body (no
	// preamble), so Split, which reads the boundary from the body's first
	// line, parses the body as the vendor will.
	Multipart bool
	Split     func(body []byte) ([]Item, error)
	// Join rebuilds the upstream body from the original body and the items in
	// Split order; only items whose Body changed need rewriting.
	Join func(original []byte, items []Item) ([]byte, error)
}

// BodyRef points at a resource ID inside a JSON body. Path is a dotted path;
// a "[]" suffix on a segment iterates an array ("data[].id").
type BodyRef struct {
	Path     string
	Resource string
	// Child marks, in ResponseIDs of an operation on an async resource, an
	// ID of a resource derived from it (a batch's output file). The node
	// that holds the async resource's ledger row records the child there
	// too (ParentID), so it can be listed and retrieved, and cannot be
	// deleted before its parent is accounted.
	Child bool
	// ObjectID marks a field that holds the ID itself or an object whose
	// "id" holds it (OpenAI's conversation: "conv_..." or {"id": ...}).
	ObjectID bool
	// Screened marks, in RequestRefs, a reference to a resource whose
	// content the gateway screened when an admitting operation created it
	// (a batch input file) and that this operation runs at the vendor. The
	// gateway refuses the reference unless the LLM's request filters are
	// still the ones that screened it.
	Screened bool
}

// TransformEnv is what a Transform may use.
type TransformEnv struct {
	// PublicBaseURL is the configured absolute base URL of this gateway's
	// native route for the LLM; empty means emit root-relative URLs.
	PublicBaseURL string
	// Wrap wraps a raw vendor ID of the named resource type for the calling
	// App and LLM.
	Wrap func(resourceType, vendorID string) (string, error)
}

// ResultView is one parsed results line.
type ResultView struct {
	// Key is the line's item key (custom_id).
	Key string
	// Model is the model that produced the line, if any.
	Model string
	// Message is the response body response filters inspect; nil if the line
	// has no message (errored, canceled, expired).
	Message []byte
	Usage   Usage
}

// ResultsSpec describes a JSONL results download.
type ResultsSpec struct {
	View func(line []byte) (ResultView, error)
	// Blocked returns the replacement line for a line a response filter
	// blocked, keeping the item key.
	Blocked func(line []byte, msg string) ([]byte, error)
}

// ListSpec describes a ledger-served listing.
type ListSpec struct {
	// Envelope renders a page of (already wrapped) objects.
	Envelope func(items [][]byte, firstID, lastID string, hasMore bool) ([]byte, error)
	// Query parameter names for the page size and keyset cursors.
	LimitParam, AfterParam, BeforeParam string
	DefaultLimit, MaxLimit              int
}

// UpstreamSpec controls what of the client request reaches the vendor.
type UpstreamSpec struct {
	// QueryAllow and HeaderAllow are the only query parameters and headers
	// forwarded (header names are canonicalised).
	QueryAllow  []string
	HeaderAllow []string
	// LongRunning marks responses that may stream for a long time (results);
	// the proxy uses an idle timeout instead of the request timeout.
	LongRunning bool
	// RefuseQuery are query parameters the gateway does not support on the
	// operation: a request carrying one with a value other than "false" is
	// refused with 400 (OpenAI's ?stream=true on a response retrieve would
	// expect a stream the operation does not serve).
	RefuseQuery []string
}

// Operation is one native endpoint.
type Operation struct {
	Name   string
	Method string
	// Path is the canonical vendor path pattern. {name} matches one
	// non-empty segment.
	Path string
	// PathResource is the ResourceType of the {id} path parameter.
	PathResource string
	Ledger       LedgerEffect
	// Budget applies the same budget pre-check as synchronous calls.
	Budget bool
	Admit  *AdmitSpec
	// RequestRefs are IDs in the request body to verify and unwrap;
	// ResponseIDs are IDs in the response body to wrap.
	RequestRefs []BodyRef
	ResponseIDs []BodyRef
	Transform   func(obj map[string]any, env TransformEnv)
	Results     *ResultsSpec
	List        *ListSpec
	Upstream    UpstreamSpec
	// ErrorShape renders a vendor-shaped error body.
	ErrorShape func(status int, typ, msg string) []byte
}

// IDParam is the path parameter bound to PathResource.
const IDParam = "id"

var validMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true,
}

// Validate checks a Spec for internal consistency. NewMatcher calls it.
func (s Spec) Validate() error {
	seen := map[string]bool{}
	for _, rt := range s.Resources {
		if rt.Name == "" {
			return fmt.Errorf("nativeapi: resource with empty name")
		}
		if seen[rt.Name] {
			return fmt.Errorf("nativeapi: duplicate resource %q", rt.Name)
		}
		seen[rt.Name] = true
		if rt.IDCharset != nil {
			src := rt.IDCharset.String()
			if !strings.HasPrefix(src, "^") || !strings.HasSuffix(src, "$") {
				return fmt.Errorf("nativeapi: resource %q: IDCharset must be anchored with ^...$", rt.Name)
			}
		}
	}
	names := map[string]bool{}
	for _, op := range s.Operations {
		if op.Name == "" {
			return fmt.Errorf("nativeapi: operation %s %s has no name", op.Method, op.Path)
		}
		if names[op.Name] {
			return fmt.Errorf("nativeapi: duplicate operation name %q", op.Name)
		}
		names[op.Name] = true
		if !validMethods[op.Method] {
			return fmt.Errorf("nativeapi: operation %q: invalid method %q", op.Name, op.Method)
		}
		pat, err := compilePattern(op.Path)
		if err != nil {
			return fmt.Errorf("nativeapi: operation %q: %w", op.Name, err)
		}
		hasID := pat.hasParam(IDParam)
		switch {
		case hasID && op.PathResource == "":
			return fmt.Errorf("nativeapi: operation %q: {id} without PathResource", op.Name)
		case !hasID && op.PathResource != "":
			return fmt.Errorf("nativeapi: operation %q: PathResource without {id}", op.Name)
		}
		if op.PathResource != "" && !seen[op.PathResource] {
			return fmt.Errorf("nativeapi: operation %q: unknown PathResource %q", op.Name, op.PathResource)
		}
		for _, ref := range append(append([]BodyRef{}, op.RequestRefs...), op.ResponseIDs...) {
			if ref.Path == "" || !seen[ref.Resource] {
				return fmt.Errorf("nativeapi: operation %q: invalid body ref %+v", op.Name, ref)
			}
		}
		for _, ref := range op.RequestRefs {
			if ref.Child {
				return fmt.Errorf("nativeapi: operation %q: a request ref cannot be a child", op.Name)
			}
		}
		if op.Admit != nil && (op.Admit.Split == nil || op.Admit.Join == nil) {
			return fmt.Errorf("nativeapi: operation %q: Admit needs Split and Join", op.Name)
		}
		if op.Results != nil && (op.Results.View == nil || op.Results.Blocked == nil) {
			return fmt.Errorf("nativeapi: operation %q: Results needs View and Blocked", op.Name)
		}
		if op.List != nil && op.List.Envelope == nil {
			return fmt.Errorf("nativeapi: operation %q: List needs Envelope", op.Name)
		}
	}
	for _, rt := range s.Resources {
		if a := rt.Async; a != nil && a.ObjectUsage == nil && (a.Results == nil) != (a.LineUsage == nil) {
			return fmt.Errorf("nativeapi: resource %q: Async needs both Results and LineUsage, or ObjectUsage", rt.Name)
		}
	}
	return s.validateChainHooks(seen)
}

// validateChainHooks checks the chain hooks against the declared resources
// and operations; seen holds the resource names.
func (s Spec) validateChainHooks(seen map[string]bool) error {
	if s.ChainHooks == nil {
		return nil
	}
	keys := map[string]bool{}
	for _, h := range s.ChainHooks.Hooks {
		if h.Name == "" {
			return fmt.Errorf("nativeapi: chain hook %s %s has no name", h.Method, h.Path)
		}
		if !validMethods[h.Method] {
			return fmt.Errorf("nativeapi: chain hook %q: invalid method %q", h.Name, h.Method)
		}
		pat, err := compilePattern(h.Path)
		if err != nil {
			return fmt.Errorf("nativeapi: chain hook %q: %w", h.Name, err)
		}
		if pat.literals() != len(pat.segs) {
			return fmt.Errorf("nativeapi: chain hook %q: path must not have parameters", h.Name)
		}
		key := h.Method + " " + h.Path
		if keys[key] {
			return fmt.Errorf("nativeapi: duplicate chain hook for %s", key)
		}
		keys[key] = true
		for _, op := range s.Operations {
			if op.Method == h.Method && op.Path == h.Path {
				return fmt.Errorf("nativeapi: chain hook %q: operation %q serves the same requests", h.Name, op.Name)
			}
		}
		for _, ref := range append(append([]BodyRef{}, h.RequestRefs...), h.ResponseIDs...) {
			if ref.Path == "" || !seen[ref.Resource] {
				return fmt.Errorf("nativeapi: chain hook %q: invalid body ref %+v", h.Name, ref)
			}
		}
		if h.EventObject != "" && len(h.ResponseIDs) == 0 {
			return fmt.Errorf("nativeapi: chain hook %q: EventObject without ResponseIDs", h.Name)
		}
	}
	return nil
}
