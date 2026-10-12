package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

var (
	// ErrNativeResourceExists: a row for this LLM, resource type and vendor
	// ID is already recorded.
	ErrNativeResourceExists = errors.New("native resource already recorded")
	// ErrNativeResourceNotFound: no row matches (or it belongs to another
	// App or LLM).
	ErrNativeResourceNotFound = errors.New("native resource not found")
)

// NativeResourceStore is the node-local ledger of resources created through
// a native API extension (models.NativeResource). Every accounting
// transition is one conditional UPDATE (compare and set), so concurrent
// settlers on any replica or worker agree on a single winner without a
// read-then-write transaction.
//
// Times are stored in UTC.
type NativeResourceStore interface {
	// Create records r. AccountingState must be set (models.NativeAccounting*).
	// A duplicate (LLM, resource type, vendor ID) is ErrNativeResourceExists.
	Create(ctx context.Context, r *models.NativeResource) error
	// GetByOwner returns the row of vendorID if it was created by appID on
	// llmID, else ErrNativeResourceNotFound.
	GetByOwner(ctx context.Context, llmID, appID uint, resourceType, vendorID string) (*models.NativeResource, error)
	// List returns an App's rows of one type on one LLM, newest first, one
	// keyset page at a time.
	List(ctx context.Context, q NativeResourceListQuery) ([]models.NativeResource, error)
	// UpdateSnapshot stores the vendor's latest view of the resource.
	UpdateSnapshot(ctx context.Context, id uint, u NativeResourceSnapshot) error
	// SchedulePoll sets when the reconciler next looks at the row, and its
	// count of consecutive failed polls.
	SchedulePoll(ctx context.Context, id uint, next time.Time, failures int) error
	// Due returns up to limit rows the reconciler should look at now:
	// pending rows whose poll time has come (or was never set), and claimed
	// rows whose claim is older than lease.
	Due(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.NativeResource, error)
	// Claim takes the accounting claim on the row for token: from pending,
	// or from a claim older than lease. It reports whether this call won.
	// Terminal rows (accounted, unaccountable, lost, none) are never claimed.
	// Claiming again with the token that holds the claim succeeds (and
	// renews it), so a claim retried after an ambiguous error is not lost.
	Claim(ctx context.Context, id uint, token string, now time.Time, lease time.Duration) (bool, error)
	// Release hands a claim held by token back to pending, so it is retried.
	Release(ctx context.Context, id uint, token string) (bool, error)
	// Settle moves a row claimed by token to a terminal state
	// (models.NativeAccountingAccounted, Unaccountable or Lost). It reports
	// false when token no longer holds the claim.
	Settle(ctx context.Context, id uint, token, state string, now time.Time) (bool, error)
	// Delete removes the row (hard delete: the vendor ID can be recorded
	// again).
	Delete(ctx context.Context, id uint) error
	// SweepExpired deletes settled rows whose results expired before
	// before. Unsettled rows stay for the reconciler to mark.
	SweepExpired(ctx context.Context, before time.Time) (int64, error)
}

// NativeResourceListQuery selects one page of NativeResourceStore.List.
// AfterVendorID pages to older rows than that one, BeforeVendorID to newer
// ones (at most one of the two); a cursor the App does not own is
// ErrNativeResourceNotFound.
type NativeResourceListQuery struct {
	LLMID          uint
	AppID          uint
	ResourceType   string
	Limit          int
	AfterVendorID  string
	BeforeVendorID string
}

// NativeResourceSnapshot is the vendor's view of a resource.
type NativeResourceSnapshot struct {
	State           string
	Snapshot        string
	EndedAt         *time.Time
	ResultsExpireAt *time.Time
}

// MaxNativeResourceListLimit caps NativeResourceStore.List pages.
const MaxNativeResourceListLimit = 1000

// gormNativeResourceStore reads through read and writes through write. On
// Studio both are the Studio database; on an edge, write is the SQLite writer
// handle (see the microgateway's database.OpenWriter).
type gormNativeResourceStore struct {
	read, write *gorm.DB
}

// NewNativeResourceStore returns the ledger over read and write (write nil:
// read). It returns nil when read is nil.
func NewNativeResourceStore(read, write *gorm.DB) NativeResourceStore {
	if read == nil {
		return nil
	}
	if write == nil {
		write = read
	}
	return &gormNativeResourceStore{read: read, write: write}
}

// NativeResourceStore returns the Studio's native resource ledger, or nil
// without a database.
func (s *Service) NativeResourceStore() NativeResourceStore {
	if s == nil || s.DB == nil {
		return nil
	}
	return NewNativeResourceStore(s.DB, s.DB)
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func (st *gormNativeResourceStore) Create(ctx context.Context, r *models.NativeResource) error {
	if r == nil || r.LLMID == 0 || r.AppID == 0 || r.ResourceType == "" || r.VendorResourceID == "" {
		return fmt.Errorf("native resource: llm, app, resource type and vendor ID are required")
	}
	if r.AccountingState == "" {
		return fmt.Errorf("native resource: accounting state is required")
	}
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	} else {
		r.CreatedAt = r.CreatedAt.UTC()
	}
	r.UpdatedAt = now
	r.EndedAt, r.ResultsExpireAt = utcPtr(r.EndedAt), utcPtr(r.ResultsExpireAt)
	r.ClaimedAt, r.AccountedAt, r.NextPollAt = utcPtr(r.ClaimedAt), utcPtr(r.AccountedAt), utcPtr(r.NextPollAt)
	if err := st.write.WithContext(ctx).Create(r).Error; err != nil {
		if models.IsUniqueViolation(err) {
			return ErrNativeResourceExists
		}
		return err
	}
	return nil
}

func (st *gormNativeResourceStore) GetByOwner(ctx context.Context, llmID, appID uint, resourceType, vendorID string) (*models.NativeResource, error) {
	var r models.NativeResource
	err := st.read.WithContext(ctx).
		Where("llm_id = ? AND resource_type = ? AND vendor_resource_id = ? AND app_id = ?", llmID, resourceType, vendorID, appID).
		Take(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNativeResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (st *gormNativeResourceStore) List(ctx context.Context, q NativeResourceListQuery) ([]models.NativeResource, error) {
	if q.AfterVendorID != "" && q.BeforeVendorID != "" {
		return nil, fmt.Errorf("native resource list: after and before are exclusive")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxNativeResourceListLimit {
		limit = MaxNativeResourceListLimit
	}
	db := st.read.WithContext(ctx).
		Where("app_id = ? AND llm_id = ? AND resource_type = ?", q.AppID, q.LLMID, q.ResourceType)

	newer := false
	switch {
	case q.AfterVendorID != "":
		c, err := st.GetByOwner(ctx, q.LLMID, q.AppID, q.ResourceType, q.AfterVendorID)
		if err != nil {
			return nil, err
		}
		db = db.Where("created_at < ? OR (created_at = ? AND id < ?)", c.CreatedAt, c.CreatedAt, c.ID)
	case q.BeforeVendorID != "":
		c, err := st.GetByOwner(ctx, q.LLMID, q.AppID, q.ResourceType, q.BeforeVendorID)
		if err != nil {
			return nil, err
		}
		db = db.Where("created_at > ? OR (created_at = ? AND id > ?)", c.CreatedAt, c.CreatedAt, c.ID)
		newer = true
	}

	var rows []models.NativeResource
	if newer {
		// The page right after the cursor, read upwards, returned newest first.
		if err := db.Order("created_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
		return rows, nil
	}
	if err := db.Order("created_at DESC, id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (st *gormNativeResourceStore) UpdateSnapshot(ctx context.Context, id uint, u NativeResourceSnapshot) error {
	res := st.write.WithContext(ctx).Model(&models.NativeResource{}).Where("id = ?", id).Updates(map[string]any{
		"state":             u.State,
		"snapshot":          u.Snapshot,
		"ended_at":          utcPtr(u.EndedAt),
		"results_expire_at": utcPtr(u.ResultsExpireAt),
		"updated_at":        time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNativeResourceNotFound
	}
	return nil
}

func (st *gormNativeResourceStore) SchedulePoll(ctx context.Context, id uint, next time.Time, failures int) error {
	res := st.write.WithContext(ctx).Model(&models.NativeResource{}).Where("id = ?", id).Updates(map[string]any{
		"next_poll_at":  next.UTC(),
		"poll_failures": failures,
		"updated_at":    time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNativeResourceNotFound
	}
	return nil
}

func (st *gormNativeResourceStore) Due(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.NativeResource, error) {
	if limit <= 0 {
		limit = 100
	}
	now = now.UTC()
	var rows []models.NativeResource
	err := st.read.WithContext(ctx).
		Where("(accounting_state = ? AND (next_poll_at IS NULL OR next_poll_at <= ?)) OR (accounting_state = ? AND claimed_at < ?)",
			models.NativeAccountingPending, now, models.NativeAccountingClaimed, now.Add(-lease)).
		Order("id ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (st *gormNativeResourceStore) Claim(ctx context.Context, id uint, token string, now time.Time, lease time.Duration) (bool, error) {
	if token == "" {
		return false, fmt.Errorf("native resource claim: empty token")
	}
	now = now.UTC()
	res := st.write.WithContext(ctx).Model(&models.NativeResource{}).
		Where("id = ? AND (accounting_state = ? OR (accounting_state = ? AND (claimed_at < ? OR claim_token = ?)))",
			id, models.NativeAccountingPending, models.NativeAccountingClaimed, now.Add(-lease), token).
		Updates(map[string]any{
			"accounting_state": models.NativeAccountingClaimed,
			"claim_token":      token,
			"claimed_at":       now,
			"updated_at":       now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (st *gormNativeResourceStore) Release(ctx context.Context, id uint, token string) (bool, error) {
	res := st.write.WithContext(ctx).Model(&models.NativeResource{}).
		Where("id = ? AND accounting_state = ? AND claim_token = ?", id, models.NativeAccountingClaimed, token).
		Updates(map[string]any{
			"accounting_state": models.NativeAccountingPending,
			"claim_token":      "",
			"claimed_at":       nil,
			"updated_at":       time.Now().UTC(),
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (st *gormNativeResourceStore) Settle(ctx context.Context, id uint, token, state string, now time.Time) (bool, error) {
	switch state {
	case models.NativeAccountingAccounted, models.NativeAccountingUnaccountable, models.NativeAccountingLost:
	default:
		return false, fmt.Errorf("native resource settle: %q is not a settled state", state)
	}
	now = now.UTC()
	res := st.write.WithContext(ctx).Model(&models.NativeResource{}).
		Where("id = ? AND accounting_state = ? AND claim_token = ?", id, models.NativeAccountingClaimed, token).
		Updates(map[string]any{
			"accounting_state": state,
			"accounted_at":     now,
			"next_poll_at":     nil,
			"updated_at":       now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (st *gormNativeResourceStore) Delete(ctx context.Context, id uint) error {
	res := st.write.WithContext(ctx).Where("id = ?", id).Delete(&models.NativeResource{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNativeResourceNotFound
	}
	return nil
}

func (st *gormNativeResourceStore) SweepExpired(ctx context.Context, before time.Time) (int64, error) {
	res := st.write.WithContext(ctx).
		Where("results_expire_at < ? AND accounting_state IN ?", before.UTC(), []string{
			models.NativeAccountingNone, models.NativeAccountingAccounted,
			models.NativeAccountingUnaccountable, models.NativeAccountingLost,
		}).
		Delete(&models.NativeResource{})
	return res.RowsAffected, res.Error
}
