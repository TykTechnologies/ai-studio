package guardrails

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

// ErrCallBudget is a request that needs more remote provider calls than the
// budget its caller set allows (WithCallBudget). It is not a provider
// failure and not unscanned text: the caller refuses the request as a
// whole and says why.
var ErrCallBudget = errors.New("guardrail call budget exhausted")

type callBudgetKey struct{}

type callBudget struct {
	limit int64
	used  atomic.Int64
}

// WithCallBudget returns ctx with a budget of limit remote provider calls,
// shared by every guardrail run under it: a native batch upload screens its
// items one by one, and without it a batch of N items could make up to N
// times the per-request piece budget in billed vendor calls. Calls answered
// from the verdict cache and calls to local providers do not count. A limit
// of 0 or less sets no budget.
func WithCallBudget(ctx context.Context, limit int) context.Context {
	if limit <= 0 {
		return ctx
	}
	return context.WithValue(ctx, callBudgetKey{}, &callBudget{limit: int64(limit)})
}

// TakeCall reserves one remote provider call from ctx's budget. It returns
// an error wrapping ErrCallBudget once the budget is spent, and nil when ctx
// carries none.
func TakeCall(ctx context.Context) error {
	b, ok := ctx.Value(callBudgetKey{}).(*callBudget)
	if !ok {
		return nil
	}
	if b.used.Add(1) > b.limit {
		return fmt.Errorf("%w: the request needs more than the %d guardrail provider calls it may make", ErrCallBudget, b.limit)
	}
	return nil
}
