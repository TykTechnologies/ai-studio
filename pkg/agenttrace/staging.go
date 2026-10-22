package agenttrace

import (
	"errors"
	"fmt"
	"sync/atomic"

	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"google.golang.org/protobuf/proto"
)

// Sealer encrypts staged capture events at rest (D4: payloads are encrypted
// at the hub with AES-256-GCM). The Enterprise key provider implements it;
// the sealed form carries its own key id so a rotated key still opens old
// rows. Implementations use stdlib crypto only (FIPS rule).
type Sealer interface {
	Seal(plain, aad []byte) ([]byte, error)
	Open(sealed, aad []byte) ([]byte, error)
}

type sealerHolder struct{ s Sealer }

var currentSealer atomic.Pointer[sealerHolder]

// SetSealer installs the process-wide Sealer (the key provider calls it at
// start-up). Nil removes it.
func SetSealer(s Sealer) {
	if s == nil {
		currentSealer.Store(nil)
		return
	}
	currentSealer.Store(&sealerHolder{s: s})
}

// CurrentSealer returns the installed Sealer, or nil.
func CurrentSealer() Sealer {
	if h := currentSealer.Load(); h != nil {
		return h.s
	}
	return nil
}

// Staged blob formats: the first byte of agent_trace_capture_events.event.
const (
	stagedPlain  byte = 0 // marshalled CaptureEventProto, payload data stripped
	stagedSealed byte = 1 // Sealer.Seal(marshalled CaptureEventProto, aad)
)

// ErrStagedSealed is returned by DecodeStaged for a sealed row when no
// Sealer is installed.
var ErrStagedSealed = errors.New("agenttrace: staged event is sealed and no sealer is installed")

func stagingAAD(idempotencyKey string) []byte {
	return []byte("agent_trace_staging|" + idempotencyKey)
}

// EncodeStaged turns an event into the staging table's blob. With a Sealer
// the whole event is sealed, bound to its idempotency key. Without one it
// fails closed: payload data is stripped (parts become content_state
// dropped) so no content ever reaches the table in plaintext; stripped
// reports whether that happened.
//
// The event must have its idempotency key set. EncodeStaged may modify the
// event's parts when it strips them.
func EncodeStaged(e *Event) (blob []byte, stripped bool, err error) {
	if e.IdempotencyKey == "" {
		return nil, false, errors.New("agenttrace: staged event has no idempotency key")
	}
	// Raw content never reaches the table: it goes through the content
	// pipeline first (or is dropped without one).
	ProcessContent(e)
	s := CurrentSealer()
	if s == nil {
		stripped = StripPayloadData(e)
	}
	plain, err := proto.Marshal(ToProto(e))
	if err != nil {
		return nil, stripped, fmt.Errorf("agenttrace: marshal capture event: %w", err)
	}
	if s == nil {
		return append([]byte{stagedPlain}, plain...), stripped, nil
	}
	sealed, err := s.Seal(plain, stagingAAD(e.IdempotencyKey))
	if err != nil {
		return nil, false, fmt.Errorf("agenttrace: seal capture event: %w", err)
	}
	return append([]byte{stagedSealed}, sealed...), false, nil
}

// DecodeStaged reverses EncodeStaged for the row with idempotencyKey.
func DecodeStaged(blob []byte, idempotencyKey string) (*Event, error) {
	if len(blob) == 0 {
		return nil, errors.New("agenttrace: empty staged event")
	}
	body := blob[1:]
	switch blob[0] {
	case stagedPlain:
	case stagedSealed:
		s := CurrentSealer()
		if s == nil {
			return nil, ErrStagedSealed
		}
		plain, err := s.Open(body, stagingAAD(idempotencyKey))
		if err != nil {
			return nil, fmt.Errorf("agenttrace: open staged event: %w", err)
		}
		body = plain
	default:
		return nil, fmt.Errorf("agenttrace: unknown staged event format %d", blob[0])
	}
	var p pb.CaptureEventProto
	if err := proto.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("agenttrace: unmarshal staged event: %w", err)
	}
	return FromProto(&p), nil
}

// StripPayloadData removes payload content, keeping each part's metadata
// and fingerprint with content_state dropped (spec §10 drop policy, step 1).
// Parts without content (metadata, hashed) are left as they are. It reports
// whether anything was removed.
func StripPayloadData(e *Event) bool {
	stripped := false
	for i := range e.Parts {
		if e.Parts[i].Data != nil {
			e.Parts[i].Data = nil
			e.Parts[i].ContentState = StateDropped
			stripped = true
		}
	}
	return stripped
}

// StripGatewayEventDetail removes gateway event details (spec §10 drop
// policy, step 2). It reports whether anything was removed.
func StripGatewayEventDetail(e *Event) bool {
	stripped := false
	for i := range e.GatewayEvents {
		if e.GatewayEvents[i].Detail != "" {
			e.GatewayEvents[i].Detail = ""
			stripped = true
		}
	}
	return stripped
}
