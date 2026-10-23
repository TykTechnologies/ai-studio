package services

import (
	"sync"
	"time"
)

// LLMBudgetSpend keeps the spend of each LLM with a monthly budget in its
// budget period (ap6): the control plane's figure from the budget sync, which
// counts Studio and every edge as of the sync, and what this gateway has
// recorded in the period since it started. The budget check takes the larger,
// as the App ledger takes the larger of the synced and the local figure.
type LLMBudgetSpend struct {
	mu   sync.Mutex
	llms map[uint]*llmSpend
	now  func() time.Time
}

// llmSpend is one LLM's spend in dollars in the period [start, end].
type llmSpend struct {
	start, end time.Time
	control    float64
	local      float64
}

// edgeLLMBudgetSpend is shared by the budget sync handler and the budget
// service, like the App ledger.
var edgeLLMBudgetSpend = NewLLMBudgetSpend()

// NewLLMBudgetSpend returns an empty set.
func NewLLMBudgetSpend() *LLMBudgetSpend {
	return &LLMBudgetSpend{llms: map[uint]*llmSpend{}, now: time.Now}
}

// current returns the LLM's entry for the period at now. Until a sync names
// an LLM's period, the calendar month stands in for it; an entry whose
// period has ended starts again. s.mu is held.
func (s *LLMBudgetSpend) current(llmID uint, now time.Time) *llmSpend {
	e := s.llms[llmID]
	if e == nil || now.After(e.end) {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		e = &llmSpend{start: start, end: start.AddDate(0, 1, 0).Add(-time.Second)}
		s.llms[llmID] = e
	}
	return e
}

// Spent returns the LLM's spend in dollars in its current period.
func (s *LLMBudgetSpend) Spent(llmID uint) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.current(llmID, s.now())
	return max(e.control, e.local)
}

// Record adds a request's cost, in dollars, to the LLM's local spend.
func (s *LLMBudgetSpend) Record(llmID uint, dollars float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current(llmID, s.now()).local += dollars
}

// Sync takes the control plane's figures. An LLM it does not name has spent
// nothing in its period as far as the control plane knows. A new period
// (a new month, a budget reset) starts the local spend again.
func (s *LLMBudgetSpend) Sync(budgets map[uint32]AppBudgetData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.llms {
		e.control = 0
	}
	for id, b := range budgets {
		e := s.llms[uint(id)]
		if e == nil || !e.start.Equal(b.PeriodStart) || !e.end.Equal(b.PeriodEnd) {
			e = &llmSpend{start: b.PeriodStart, end: b.PeriodEnd}
			s.llms[uint(id)] = e
		}
		e.control = b.Usage
	}
}
