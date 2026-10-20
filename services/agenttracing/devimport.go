package agenttracing

import "context"

// DevImporter stores a JSON v1 session document as trace rows: the e2e
// seed (jyx.35). The Enterprise service implements it; the API exposes it
// only in DEVMODE, so production installs never offer it.
type DevImporter interface {
	// ImportDocument stores doc (one JSON v1 session document) for an App
	// in the viewer's scope and returns its session id. Its content goes
	// through the content pipeline like captured traffic; the import is
	// audited (and refused when it cannot be).
	ImportDocument(ctx context.Context, v Viewer, m AccessMeta, doc []byte) (sessionID string, err error)
}
