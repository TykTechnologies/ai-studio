package services

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/pkg/replicas"
)

// spendRecalculated is called after a price recalculation has committed new
// costs for stored chat records. Budget spend kept in memory was summed from
// the old costs and only ever grows, so without this a raised price was not
// enforced and a lowered one kept an App blocked until a restart, a cache
// eviction or the next period. It clears this replica's budget and team
// budget caches at once, tells the other replicas through the cluster event
// log, and delivers the signal here too, for this replica's hub budget sync.
func (s *Service) spendRecalculated() {
	if s.Budget != nil {
		s.Budget.ClearCache()
	}
	if s.TeamBudget != nil {
		s.TeamBudget.ClearCache()
	}
	replicas.Signal(context.Background(), replicas.SignalSpendRecalculated)
	replicas.Deliver(replicas.SignalSpendRecalculated)
}
