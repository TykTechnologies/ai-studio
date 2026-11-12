package eventbridge

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/pkg/credhash"
)

// CredentialChangedTopic is the DirDown event the control plane raises when
// an App credential changes (created, activated, deactivated, deleted) or
// its App is activated or deactivated (features/ActiveActive.md, Phase 4
// item 4). It is an accelerator: an edge drops what it cached for the
// credential at once instead of at the cache TTL or the next credential
// sync delta. Correctness still comes from validation and the sync.
const CredentialChangedTopic = "credential.changed"

// CredentialChanged is the payload of a CredentialChangedTopic event. It is
// shared by the control plane, which raises it, and the edge, which acts on
// it, so neither can drift from the other.
//
// It carries no key. Hash is the credential's edge-facing hash: pkg/credhash
// under the edges' MICROGATEWAY_ENCRYPTION_KEY, the hash a full credential
// sync sends and an edge's token cache is keyed by. For a credential that
// is deactivated or deleted it is computed before the change takes effect,
// so the edge can still find what it cached.
//
// An edge must only ever drop state on this event, never add or widen it:
// whether a key may be served is decided by validation (or by the sync's
// own pages), never by an event's payload.
type CredentialChanged struct {
	CredentialID uint   `json:"credential_id"`
	Hash         string `json:"hash"`
	AppID        uint   `json:"app_id"`
	// Namespace is the App's namespace ("" for global).
	Namespace string `json:"namespace"`
	Active    bool   `json:"active"`
	Deleted   bool   `json:"deleted"`
}

// ErrCredentialChangedNoHash is returned for a payload that names no
// credential hash: the edge has nothing to find.
var ErrCredentialChangedNoHash = errors.New("credential.changed event without a credential hash")

// ErrCredentialChangedBadHash is returned for a payload whose hash is not
// the length of a credential hash (credhash.Len): it cannot name a key.
var ErrCredentialChangedBadHash = errors.New("credential.changed event with a hash that is not a credential hash")

// ParseCredentialChanged decodes a CredentialChangedTopic payload. A
// payload with no hash, or one that is not credhash.Len long, is refused.
func ParseCredentialChanged(payload json.RawMessage) (CredentialChanged, error) {
	var c CredentialChanged
	if err := json.Unmarshal(payload, &c); err != nil {
		return CredentialChanged{}, fmt.Errorf("decode credential.changed event: %w", err)
	}
	if c.Hash == "" {
		return CredentialChanged{}, ErrCredentialChangedNoHash
	}
	if len(c.Hash) != credhash.Len {
		return CredentialChanged{}, fmt.Errorf("%w (%d characters, not %d)", ErrCredentialChangedBadHash, len(c.Hash), credhash.Len)
	}
	return c, nil
}
