package agenttracing

import "context"

// Export formats (spec §7, MVP).
const (
	ExportJSON     = "json"
	ExportMarkdown = "markdown"
)

// ExportQuery selects an export's format and whether it carries content.
// Content is included only when asked for AND the viewer may read it; the
// result and its audit record say which.
type ExportQuery struct {
	Format  string
	Content bool
}

// ExportResult is a rendered export, served as an attachment.
type ExportResult struct {
	ContentType string
	Filename    string
	Body        []byte
	// ContentIncluded reports whether captured content is in Body.
	ContentIncluded bool
}

// Exporter renders session and trace exports. Every export is audited
// before it is returned, and refused (ErrAuditUnavailable) when it cannot
// be.
type Exporter interface {
	ExportSession(ctx context.Context, v Viewer, sessionID string, q ExportQuery, m AccessMeta) (*ExportResult, error)
	ExportTrace(ctx context.Context, v Viewer, appID uint, traceID []byte, q ExportQuery, m AccessMeta) (*ExportResult, error)
}

func (s communityService) ExportSession(context.Context, Viewer, string, ExportQuery, AccessMeta) (*ExportResult, error) {
	return nil, s.err
}

func (s communityService) ExportTrace(context.Context, Viewer, uint, []byte, ExportQuery, AccessMeta) (*ExportResult, error) {
	return nil, s.err
}
