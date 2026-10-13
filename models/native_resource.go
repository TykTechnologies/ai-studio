package models

import "time"

// Accounting states of a NativeResource. A resource whose usage is billed
// later (an Anthropic message batch) starts pending and is accounted exactly
// once: whoever settles it (a results download or the reconciler) first
// claims it, then marks it with one of the terminal states.
const (
	// NativeAccountingNone: nothing to account (a resource that carries no
	// usage of its own).
	NativeAccountingNone = "none"
	// NativeAccountingPending: usage not recorded yet.
	NativeAccountingPending = "pending"
	// NativeAccountingClaimed: a settler holds the claim (ClaimToken,
	// ClaimedAt). A claim older than the lease can be taken over.
	NativeAccountingClaimed = "claimed"
	// NativeAccountingAccounted: usage recorded. Never claimable again.
	NativeAccountingAccounted = "accounted"
	// NativeAccountingUnaccountable: the results expired at the vendor
	// before they were read; the usage is lost.
	NativeAccountingUnaccountable = "unaccountable"
	// NativeAccountingLost: the vendor no longer knows the resource.
	NativeAccountingLost = "lost"
)

// NativeAccountingTerminal reports whether state is final: no claim can be
// taken on it.
func NativeAccountingTerminal(state string) bool {
	switch state {
	case NativeAccountingNone, NativeAccountingAccounted, NativeAccountingUnaccountable, NativeAccountingLost:
		return true
	}
	return false
}

// NativeResource is the node-local ledger row of a vendor resource created
// through a native API extension (see pkg/nativeapi), such as an Anthropic
// message batch. Studio replicas share theirs in the Studio database; every
// edge keeps its own in its SQLite. Only the node that created a resource
// has its row, and only that node accounts for it.
//
// Rows are deleted outright (no soft delete): the vendor ID is unique per
// LLM and resource type, and a tombstone would block re-creating it.
type NativeResource struct {
	ID uint `gorm:"primaryKey;index:idx_native_resources_owner_keyset,priority:5"`

	// Vendor is the LLM's vendor (models.Vendor) when the row was created.
	Vendor string `gorm:"size:64;not null"`
	LLMID  uint   `gorm:"not null;uniqueIndex:idx_native_resources_vendor_id,priority:1;index:idx_native_resources_owner_keyset,priority:2"`
	AppID  uint   `gorm:"not null;index:idx_native_resources_owner_keyset,priority:1"`
	UserID uint
	// OnBehalfOf and ActingAgent are the identity stamps of the creating
	// request, so usage accounted later without a request carries them.
	OnBehalfOf  string `gorm:"size:255"`
	ActingAgent string `gorm:"size:255"`

	// ResourceType is the extension's resource type name, such as
	// "anthropic.message_batch".
	ResourceType     string `gorm:"size:128;not null;uniqueIndex:idx_native_resources_vendor_id,priority:2;index:idx_native_resources_owner_keyset,priority:3"`
	VendorResourceID string `gorm:"size:255;not null;uniqueIndex:idx_native_resources_vendor_id,priority:3"`
	// ParentID is the row of the resource this one was derived from (an
	// output file of a batch), if any.
	ParentID *uint `gorm:"index"`
	// ScreeningDigest identifies the request filters that screened the
	// resource's content when an admitting operation created it (a batch
	// input file): "none" when the LLM had none. An operation that later
	// runs that content (a batch create) is refused unless the LLM's
	// request filters still have this digest. Empty on rows written before
	// the column existed and on resources nothing screened. Never returned
	// to clients.
	ScreeningDigest string `gorm:"size:64"`

	// State is the vendor's processing state as the extension reports it
	// (in_progress, canceling, ended).
	State string `gorm:"size:32"`
	// Snapshot is the last vendor object returned to the client, with its
	// IDs already wrapped; list operations serve it.
	Snapshot        string `gorm:"type:text"`
	EndedAt         *time.Time
	ResultsExpireAt *time.Time `gorm:"index"`

	AccountingState string `gorm:"size:16;not null;index:idx_native_resources_due,priority:1"`
	ClaimToken      string `gorm:"size:64"`
	ClaimedAt       *time.Time
	AccountedAt     *time.Time
	NextPollAt      *time.Time `gorm:"index:idx_native_resources_due,priority:2"`
	PollFailures    int        `gorm:"not null;default:0"`

	CreatedAt time.Time `gorm:"index:idx_native_resources_owner_keyset,priority:4"`
	UpdatedAt time.Time
}
