package models

import (
	"sync"

	"gorm.io/gorm"
)

// TokenTotals keeps running sums of llm_chat_records.total_tokens, overall
// and per interaction type, for usage telemetry.
//
// Telemetry used to run SUM(total_tokens) over the whole table three times
// at start-up and every hour. At ~100M rows each sum is a full scan of the
// largest table, which on a busy hub kept the database busy for minutes. The
// tracker sums the table once, in one grouped pass, and afterwards reads only
// the rows added since, found by id. llm_chat_records rows are only ever
// inserted, never updated or deleted.
//
// Rows are folded into the settled totals one read after they were first
// seen, so a row whose transaction commits after rows with higher ids (several
// hub replicas writing to one database) is still counted if it commits before
// the next read.
type TokenTotals struct {
	mu          sync.Mutex
	initialized bool
	// Rows with id <= settledID are included in settled.
	settledID uint
	// pendingID is the highest id seen at the last read.
	pendingID uint
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
func NewTokenTotals() *TokenTotals { return &TokenTotals{} }

// Read returns the tokens of every llm_chat_records row, overall and by
// interaction type.
func (t *TokenTotals) Read(db *gorm.DB) (all int64, byType map[InteractionType]int64, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

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
		t.settledID, t.pendingID, t.initialized = maxID, maxID, true
	} else if t.pendingID > t.settledID {
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
	if maxID > t.pendingID {
		t.pendingID = maxID
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
