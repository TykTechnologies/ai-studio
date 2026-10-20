package agenttrace

import (
	"context"
	"sync/atomic"
	"unicode/utf8"
)

// RawPart is content captured at the gateway before the content pipeline
// (§5.5): a request or response body, tool arguments or a tool result, as
// the wire carried it. It never leaves the process: the converters
// (ToProto, EncodeStaged) carry only processed Parts, and ProcessContent
// turns raw parts into processed ones in the capture worker, off the request
// path.
type RawPart struct {
	Part      string // PartRequest, PartResponse, ...
	MediaType string
	Data      []byte
	// Bytes is the size on the wire; larger than len(Data) when the capture
	// kept only MaxRawPartBytes of it.
	Bytes int64
}

// MaxRawPartBytes caps what CaptureRaw keeps of one part, so a huge body
// cannot pin memory in the capture queue. Content over the cap is processed
// as truncated. The content pipeline truncates far below it (256 KiB by
// default); the cap only bounds memory.
var maxRawPartBytes atomic.Int64

func init() { maxRawPartBytes.Store(4 << 20) }

// SetMaxRawPartBytes sets the per-part raw capture cap (start-up).
func SetMaxRawPartBytes(n int64) {
	if n > 0 {
		maxRawPartBytes.Store(n)
	}
}

// CaptureRaw adds a raw part to the request's primary span. The data is
// copied (up to the cap), so the caller may reuse its buffer. A no-op without
// a Recorder: in off mode the hook should not even buffer the body.
func CaptureRaw(ctx context.Context, part, mediaType string, data []byte) {
	From(ctx).CaptureRaw(part, mediaType, data)
}

// CaptureRaw adds a raw part to the primary span.
func (r *Recorder) CaptureRaw(part, mediaType string, data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	keep := data
	if limit := maxRawPartBytes.Load(); int64(len(keep)) > limit {
		keep = keep[:limit]
	}
	raw := RawPart{Part: part, MediaType: mediaType, Data: append([]byte(nil), keep...), Bytes: int64(len(data))}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return
	}
	r.span.Raw = append(r.span.Raw, raw)
}

// ContentProcessor turns an event's raw parts into persisted-safe parts for
// its effective content mode (§5.5, §9.3): secret stripping, binary
// replacement, redaction, truncation, fingerprints. The Enterprise content
// pipeline implements it, at the capturing gateway (hub or edge), before the
// event is staged or sent on the pulse. Process sets ev.ContentMode and
// ev.Parts and redacts ev.ErrorMessage; ProcessContent clears ev.Raw.
type ContentProcessor interface {
	Process(ev *Event)
}

type processorHolder struct{ p ContentProcessor }

var currentProcessor atomic.Pointer[processorHolder]

// SetContentProcessor installs the process-wide ContentProcessor. Nil
// removes it.
func SetContentProcessor(p ContentProcessor) {
	if p == nil {
		currentProcessor.Store(nil)
		return
	}
	currentProcessor.Store(&processorHolder{p: p})
}

// CurrentContentProcessor returns the installed ContentProcessor, or nil.
func CurrentContentProcessor() ContentProcessor {
	if h := currentProcessor.Load(); h != nil {
		return h.p
	}
	return nil
}

// ProcessContent runs the content pipeline on ev's raw parts and clears
// them, once per event. Transports call it in their worker before encoding
// (EncodeStaged calls it too; the edge pulse transport must call it before
// ToProto). Without a ContentProcessor it fails closed: each raw part
// becomes a dropped part with its size and no data. Error messages are
// redacted only by a processor; without one an error message longer than
// maxUnprocessedError is cut.
func ProcessContent(ev *Event) {
	if ev == nil || ev.contentProcessed {
		return
	}
	ev.contentProcessed = true
	if p := CurrentContentProcessor(); p != nil {
		p.Process(ev)
		ev.Raw = nil
		return
	}
	for _, r := range ev.Raw {
		ev.Parts = append(ev.Parts, PayloadPart{Part: r.Part, ContentState: StateDropped, MediaType: r.MediaType, Bytes: r.Bytes})
	}
	ev.Raw = nil
	if len(ev.ErrorMessage) > maxUnprocessedError {
		cut := maxUnprocessedError
		for cut > 0 && !utf8.RuneStart(ev.ErrorMessage[cut]) {
			cut--
		}
		ev.ErrorMessage = ev.ErrorMessage[:cut]
	}
}

const maxUnprocessedError = 256
