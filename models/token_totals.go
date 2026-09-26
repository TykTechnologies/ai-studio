package models

import (
	"sync"
	"time"

	"gorm.io/gorm"
)

// tokenTotalsSettleAfter is how long rows must have been visible before they
// are folded into the settled totals. One telemetry collection reads the
// totals several times within a second (LLM, App and Chat stats); a window
// counted in reads rather than time would settle rows those reads had only
// just seen. The repeated reads themselves are cheap: MAX(id) on the primary
// key and a sum over the rows added since the last settle.
const tokenTotalsSettleAfter = time.Minute

// TokenTotals keeps running sums of llm_chat_records.total_tokens, overall
// and per interaction type, for usage telemetry.
//
// Telemetry used to run SUM(total_tokens) over the whole table three times
// at start-up and every hour. At ~100M rows each sum is a full scan of the
// largest table, which on a busy hub kept the database busy for minutes. The
// tracker sums the table once, in one grouped pass, and afterwards reads only
// the rows added since, found by id. This relies on llm_chat_records rows
// being insert-only (see LLMChatRecord).
//
// Rows are folded into the settled totals only once they have been visible
// for tokenTotalsSettleAfter, and until then are summed afresh on each read,
// so a row whose transaction commits after rows with higher ids (several hub
// replicas writing to one database) is still counted if it commits within
// that window.
type TokenTotals struct {
	mu          sync.Mutex
	now         func() time.Time
	initialized bool
	// Rows with id <= settledID are included in settled.
	settledID uint
	// pendingID is the highest id seen when pendingAt was taken; rows up to
	// it settle once pendingAt is tokenTotalsSettleAfter old.
	pendingID uint
	pendingAt time.Time
	settled   tokenSums
}

type tokenSums struct {
	all    int64
	byType map[InteractionType]int64
}

func (s *tokenSums) add(o tokenSums) {
	s.all += o.all
	if s.byType == nil {
		s.byType = map[InteractionType]int64{}
	}
	for k, v := range o.byType {
		s.byType[k] += v
	}
}

// NewTokenTotals returns an empty tracker.
func NewTokenTotals() *TokenTotals { return &TokenTotals{now: time.Now} }

// Read returns the tokens of every llm_chat_records row, overall and by
// interaction type.
func (t *TokenTotals) Read(db *gorm.DB) (all int64, byType map[InteractionType]int64, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	var maxID uint
	if err := db.Model(&LLMChatRecord{}).Select("COALESCE(MAX(id), 0)").Scan(&maxID).Error; err != nil {
		return 0, nil, err
	}

	if !t.initialized {
		sums, err := sumTokens(db, 0, maxID)
		if err != nil {
			return 0, nil, err
		}
		t.settled.add(sums)
		t.settledID, t.pendingID, t.pendingAt, t.initialized = maxID, maxID, now, true
	} else if t.pendingID > t.settledID && now.Sub(t.pendingAt) >= tokenTotalsSettleAfter {
		sums, err := sumTokens(db, t.settledID, t.pendingID)
		if err != nil {
			return 0, nil, err
		}
		t.settled.add(sums)
		t.settledID = t.pendingID
	}

	total := tokenSums{}
	total.add(t.settled)
	if maxID > t.settledID {
		recent, err := sumTokens(db, t.settledID, maxID)
		if err != nil {
			return 0, nil, err
		}
		total.add(recent)
	}
	// Start the next settle window once the previous one has settled.
	if t.pendingID == t.settledID && maxID > t.pendingID {
		t.pendingID, t.pendingAt = maxID, now
	}

	return total.all, total.byType, nil
}

// sumTokens sums total_tokens by interaction type for rows with
// afterID < id <= uptoID.
func sumTokens(db *gorm.DB, afterID, uptoID uint) (tokenSums, error) {
	var rows []struct {
		InteractionType InteractionType
		Total           int64
	}
	err := db.Model(&LLMChatRecord{}).
		Select("interaction_type, COALESCE(SUM(total_tokens), 0) AS total").
		Where("id > ? AND id <= ?", afterID, uptoID).
		Group("interaction_type").
		Scan(&rows).Error
	if err != nil {
		return tokenSums{}, err
	}
	s := tokenSums{byType: make(map[InteractionType]int64, len(rows))}
	for _, r := range rows {
		s.all += r.Total
		s.byType[r.InteractionType] += r.Total
	}
	return s, nil
}
