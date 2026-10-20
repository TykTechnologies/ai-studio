package proxy

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/rs/zerolog/log"
)

// streamScreen is a response writer that screens a stream a bridge writes
// (the /ai/ and /anthropic/ bridges to Bedrock, which re-encode a Converse
// stream as OpenAI or Anthropic SSE) the way the native streaming loop
// screens a vendor's stream: it frames what is written into whole events,
// runs the LLM's response filters over their text, holds tool calls until
// they are classified when a response guardrail scans them, and releases
// only what passed. A block ends the stream with the dialect's error event;
// the bridge checks Blocked and stops. Finish runs the end-of-stream pass.
type streamScreen struct {
	w       http.ResponseWriter
	flusher http.Flusher
	p       *Proxy
	llm     *models.LLM
	r       *http.Request
	reqBody []byte
	// reqFormat is reqBody's format, for the goal of held tool calls.
	reqFormat string

	dialect    streamDialect
	framer     *eventFramer
	hold       *toolCallHold
	text       strings.Builder
	chunkIndex int
	blocked    bool
	gone       bool
	// pending is the block message of held tool calls a filter blocked.
	pending string
}

func (p *Proxy) newStreamScreen(w http.ResponseWriter, flusher http.Flusher, r *http.Request, llm *models.LLM, dialect streamDialect, contentType string, reqBody []byte, reqFormat string) *streamScreen {
	s := &streamScreen{w: w, flusher: flusher, p: p, llm: llm, r: r, reqBody: reqBody, reqFormat: reqFormat,
		dialect: dialect, framer: newEventFramer(contentType)}
	if responseFiltersScanToolCalls(llm) {
		s.hold = newToolCallHold(dialect)
	}
	return s
}

func (s *streamScreen) Header() http.Header { return s.w.Header() }

func (s *streamScreen) WriteHeader(code int) { s.w.WriteHeader(code) }

func (s *streamScreen) Flush() {
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// Blocked reports that a filter ended the stream.
func (s *streamScreen) Blocked() bool { return s.blocked }

// Text is the response text seen so far.
func (s *streamScreen) Text() string { return s.text.String() }

func (s *streamScreen) Write(b []byte) (int, error) {
	if s.blocked || s.gone {
		return len(b), nil
	}
	events := s.framer.feed(b)
	var delta strings.Builder
	var release []byte
	for _, ev := range events {
		delta.WriteString(s.dialect.text(ev))
		if s.hold == nil {
			release = append(release, ev.raw...)
			continue
		}
		now, ready := s.hold.observe(ev)
		release = append(release, now...)
		if ready {
			held, ok := s.classifyHeld()
			if !ok {
				s.emit(release)
				s.block()
				return len(b), nil
			}
			release = append(release, held...)
		}
	}
	s.text.WriteString(delta.String())
	if len(events) > 0 {
		blocked, msg, err := ExecuteStreamTextFilters(s.llm, s.p.gatewayService, delta.String(), http.StatusOK, s.chunkIndex, s.text.String(), s.r)
		if err != nil {
			log.Error().Err(err).Int("chunk", s.chunkIndex).Msg("Response filter error on a bridged stream")
		} else if blocked {
			s.blockWith(msg)
			return len(b), nil
		}
		s.chunkIndex++
	}
	s.emit(release)
	return len(b), nil
}

// classifyHeld classifies the held calls; ok is false when they are blocked
// (the block message is kept for block()).
func (s *streamScreen) classifyHeld() ([]byte, bool) {
	held, calls, overflow := s.hold.take()
	blocked, msg, err := classifyHeldCalls(s.llm, s.p.gatewayService, calls, overflow, s.reqBody, s.reqFormat, http.StatusOK, s.r)
	if err != nil {
		log.Error().Err(err).Msg("Tool call filter error on a bridged stream")
	}
	if blocked {
		s.pending = msg
		return nil, false
	}
	return held, true
}

func (s *streamScreen) block() { s.blockWith(s.pending) }

func (s *streamScreen) blockWith(msg string) {
	recordPolicyBlock(s.r.Context(), "response_filter", "filter")
	log.Warn().Str("reason", msg).Msg("Bridged stream blocked by filter")
	s.emit(s.dialect.blockEvent(s.framer, msg))
	s.blocked = true
}

func (s *streamScreen) emit(b []byte) {
	if len(b) == 0 || s.gone {
		return
	}
	if _, err := s.w.Write(b); err != nil {
		s.gone = true
		return
	}
	s.Flush()
}

// Finish ends the stream's screening: the last partial event, tool calls
// still held, then the final pass over the whole text. It reports whether
// the stream was blocked (then the bridge must not write any more).
func (s *streamScreen) Finish() bool {
	if s.blocked {
		return true
	}
	var tail []byte
	if ev := s.framer.flush(); ev != nil {
		s.text.WriteString(s.dialect.text(*ev))
		tail = ev.raw
		if s.hold != nil {
			tail, _ = s.hold.observe(*ev)
		}
	}
	if s.hold != nil && s.hold.holding {
		held, ok := s.classifyHeld()
		if !ok {
			s.block()
			return true
		}
		tail = append(held, tail...)
	}
	blocked, msg, err := ExecuteFinalResponseFilters(s.llm, s.p.gatewayService, http.StatusOK, s.chunkIndex, s.text.String(), s.r)
	if err != nil {
		log.Error().Err(err).Msg("Response filter error at the end of a bridged stream")
	} else if blocked {
		s.blockWith(msg)
		return true
	}
	s.emit(tail)
	return false
}
