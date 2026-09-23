package services

import (
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// BudgetBlocks holds the Apps the control plane says this gateway must
// refuse on budget grounds (Enterprise): their Studio budget is 0, or their
// team has spent a hard-blocking team budget. Studio budgets distinguish
// "no limit" from 0, but the synced App config (and this gateway's own App
// table) still read a budget at or below 0 as "no limit", so a zero budget
// reaches the gateway this way rather than through the App config.
//
// The budget sync replaces the whole set every interval, so an App is
// released within one sync of being allowed again. The set is persisted in
// budget_blocks so a restarted gateway keeps refusing until the next sync.
type BudgetBlocks struct {
	mu     sync.RWMutex
	blocks map[uint]string
}

// edgeBudgetBlocks is shared by the budget sync handler (writer) and the
// Enterprise budget check (reader).
var edgeBudgetBlocks = &BudgetBlocks{blocks: map[uint]string{}}

// Replace swaps in a complete set of blocks.
func (b *BudgetBlocks) Replace(blocks map[uint]string) {
	next := make(map[uint]string, len(blocks))
	for id, reason := range blocks {
		next[id] = reason
	}
	b.mu.Lock()
	b.blocks = next
	b.mu.Unlock()
}

// Reason returns why an App is blocked, and whether it is.
func (b *BudgetBlocks) Reason(appID uint) (string, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	reason, ok := b.blocks[appID]
	return reason, ok
}

// loadBudgetBlocks fills the set from the persisted table.
func loadBudgetBlocks(db *gorm.DB, into *BudgetBlocks) {
	var rows []database.BudgetBlock
	if err := db.Find(&rows).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to load persisted budget blocks")
		return
	}
	blocks := make(map[uint]string, len(rows))
	for _, r := range rows {
		blocks[r.AppID] = r.Reason
	}
	into.Replace(blocks)
}

// persistBudgetBlocks replaces the persisted table with the given set.
func persistBudgetBlocks(db *gorm.DB, blocks map[uint]string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&database.BudgetBlock{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for id, reason := range blocks {
			if err := tx.Create(&database.BudgetBlock{AppID: id, Reason: reason, UpdatedAt: now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
