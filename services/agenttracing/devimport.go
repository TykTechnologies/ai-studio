package agenttracing

import "context"

// DevImporter stores a JSON v1 session document as trace rows: the e2e
// seed (jyx.35). The Enterprise service implements it; the API exposes it
// only in DEVMODE, so production installs never offer it.
type DevImporter interface {
	// ImportDocument stores doc (one JSON v1 session document) and returns
	// its session id. Captured content in the document is sealed like
	// captured traffic.
	ImportDocument(ctx context.Context, doc []byte) (sessionID string, err error)
}
