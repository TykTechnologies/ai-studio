package agenttracing

import (
	"context"
	"errors"
)

// ErrAuditUnavailable: the audit record for a content access could not be
// written, so the content is not served (spec §9.2, fail closed). The API
// answers 503.
var ErrAuditUnavailable = errors.New("agent trace content is unavailable: the access could not be audited")

// ErrForbidden: the viewer lacks the permission for this operation on this
// App. The API answers 403 (or 404 where existence must not leak).
var ErrForbidden = errors.New("agent trace access denied")

// AccessMeta is the request context an audited access records (spec §9.2).
type AccessMeta struct {
	ClientIP  string
	UserAgent string
	RequestID string
	UserEmail string
	UserName  string
}

// PayloadQuery selects one captured payload part of a span.
type PayloadQuery struct {
	Part string
	// MaxBytes caps the returned content (the UI's view cap applies too);
	// content over it comes back truncated, with state truncated.
	MaxBytes int
}

// ContentService is the content half of the API (spec §8, §9.2, §9.4):
// payload reads, purge and settings. Every content read writes an audit
// record before it returns content, whatever AUDIT_RECORD_READS says, and
// returns ErrAuditUnavailable when it cannot.
type ContentService interface {
	GetPayload(ctx context.Context, v Viewer, appID uint, spanID []byte, q PayloadQuery, m AccessMeta) (any, error)
	// PurgeSession deletes a session and everything captured for it.
	PurgeSession(ctx context.Context, v Viewer, sessionID string, m AccessMeta) (any, error)
	// PurgeEndUser deletes every session of a declared end user (right to
	// erasure). The end user arrives in a request body, never a URL.
	PurgeEndUser(ctx context.Context, v Viewer, endUser string, m AccessMeta) (any, error)
	// GetConfig returns the effective tracing settings.
	GetConfig(ctx context.Context, v Viewer) (any, error)
}

func (s communityService) GetPayload(context.Context, Viewer, uint, []byte, PayloadQuery, AccessMeta) (any, error) {
	return nil, s.err
}

func (s communityService) PurgeSession(context.Context, Viewer, string, AccessMeta) (any, error) {
	return nil, s.err
}

func (s communityService) PurgeEndUser(context.Context, Viewer, string, AccessMeta) (any, error) {
	return nil, s.err
}

func (s communityService) GetConfig(context.Context, Viewer) (any, error) { return nil, s.err }
