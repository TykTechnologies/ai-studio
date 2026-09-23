package services

import (
	"sync"
)

// TeamBlocks holds the Apps the control plane says this gateway must refuse
// because of their team (Enterprise team budgets): an App with no team
// allocation, or one in a hard-blocking team that has reached its budget.
// The budget sync replaces the whole set every interval, so an App is
// released within one sync of its team getting back under budget. The set
// is in memory only; after a restart it is empty until the first sync.
type TeamBlocks struct {
	mu     sync.RWMutex
	blocks map[uint]string
}

// edgeTeamBlocks is shared by the budget sync handler (writer) and the
// Enterprise budget check (reader).
var edgeTeamBlocks = &TeamBlocks{blocks: map[uint]string{}}

// Replace swaps in a complete set of blocks.
func (t *TeamBlocks) Replace(blocks map[uint]string) {
	next := make(map[uint]string, len(blocks))
	for id, reason := range blocks {
		next[id] = reason
	}
	t.mu.Lock()
	t.blocks = next
	t.mu.Unlock()
}

// Reason returns why an App is blocked, and whether it is.
func (t *TeamBlocks) Reason(appID uint) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	reason, ok := t.blocks[appID]
	return reason, ok
}
