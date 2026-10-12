package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/TykTechnologies/midsommar/v2/analytics"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/nativeapi"
	"github.com/TykTechnologies/midsommar/v2/services"
)

// Accounting of async native resources (an Anthropic batch): the usage is
// only known from the results file, so it is recorded when the results are
// read, by a download on the node that created the resource or by the
// reconciler (native_reconciler.go). Either settles the ledger row first
// (claim, then settle as accounted) and records only if it won, so a batch
// is accounted exactly once - at most once if the node dies between the
// settle and the analytics hand-off, as the design accepts.

// nativeClaimLease is how long an accounting claim holds before another
// settler may take it over.
const nativeClaimLease = 5 * time.Minute

// nativeUsageKey groups usage that is priced the same.
type nativeUsageKey struct {
	model      string
	tier       string
	multiplier float64
}

// nativeUsage aggregates the billed usage of one resource's results.
type nativeUsage struct {
	order []nativeUsageKey
	byKey map[nativeUsageKey]*nativeUsageTotal
}

type nativeUsageTotal struct {
	usage    nativeapi.Usage
	requests int
}

func newNativeUsage() *nativeUsage {
	return &nativeUsage{byKey: map[nativeUsageKey]*nativeUsageTotal{}}
}

// add counts one billed result.
func (u *nativeUsage) add(x nativeapi.Usage) {
	if !x.Billed {
		return
	}
	k := nativeUsageKey{model: x.Model, tier: x.Tier, multiplier: x.PriceMultiplier()}
	t, ok := u.byKey[k]
	if !ok {
		t = &nativeUsageTotal{usage: nativeapi.Usage{Model: x.Model, Tier: x.Tier, Multiplier: x.Multiplier, Billed: true}}
		u.byKey[k] = t
		u.order = append(u.order, k)
	}
	t.usage.InputTokens += x.InputTokens
	t.usage.OutputTokens += x.OutputTokens
	t.usage.CacheCreationInputTokens += x.CacheCreationInputTokens
	t.usage.CacheReadInputTokens += x.CacheReadInputTokens
	t.requests++
}

// newClaimToken is a fresh accounting claim token.
func newClaimToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// settleNativeAccounting claims and settles row as accounted. It reports
// whether this caller won and must record the usage; a row someone else
// holds or already settled is left alone.
func settleNativeAccounting(ctx context.Context, store services.NativeResourceStore, row *models.NativeResource) (bool, error) {
	if models.NativeAccountingTerminal(row.AccountingState) {
		return false, nil
	}
	token := newClaimToken()
	won, err := claimNativeWithRetry(ctx, store, row.ID, token)
	if err != nil || !won {
		return false, err
	}
	return settleNativeWithRetry(ctx, store, row.ID, token, models.NativeAccountingAccounted)
}

// claimNativeWithRetry takes the accounting claim for token, retrying briefly
// on a store error (a busy SQLite writer, a dropped connection) like
// settleNativeWithRetry. A claim another settler holds (false, nil) is not
// retried. Retrying with the same token is safe: the store's Claim also
// matches a row this token already holds.
func claimNativeWithRetry(ctx context.Context, store services.NativeResourceStore, id uint, token string) (bool, error) {
	var err error
	for attempt := 0; attempt < nativeSettleAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, err
			case <-time.After(time.Duration(25<<attempt) * time.Millisecond):
			}
		}
		var won bool
		if won, err = store.Claim(ctx, id, token, time.Now().UTC(), nativeClaimLease); err == nil {
			return won, nil
		}
	}
	return false, err
}

// recordAsyncUsage records the usage of a settled resource: per model and
// price, one proxy log row and one chat record (priced through
// buildChatRecordFor, so the multiplier applies) through
// analytics.RecordExchange, then the budget. The records carry the creating
// request's identity from the ledger row. It never uses
// RecordChatRecordsBatch, which the edge does not pulse to the hub.
func (p *Proxy) recordAsyncUsage(row *models.NativeResource, llm *models.LLM, app *models.App, usage *nativeUsage, ts time.Time) {
	for _, k := range usage.order {
		t := usage.byKey[k]
		rec := buildChatRecordFor(p.gatewayService, llm, app, t.usage, t.usage.Model, ts)
		rec.OnBehalfOf, rec.ActingAgent = row.OnBehalfOf, row.ActingAgent
		summary, _ := json.Marshal(map[string]any{
			"native_resource": row.ResourceType,
			"model":           t.usage.Model,
			"service_tier":    t.usage.Tier,
			"requests":        t.requests,
		})
		l := &models.ProxyLog{
			AppID:        app.ID,
			UserID:       app.UserID,
			TimeStamp:    ts,
			LLMID:        llm.ID,
			Vendor:       string(llm.Vendor),
			ModelName:    llm.Name,
			ResponseBody: string(summary),
			ResponseCode: 200,
			OnBehalfOf:   row.OnBehalfOf,
			ActingAgent:  row.ActingAgent,
			ServiceTier:  rec.ServiceTier,
		}
		if llm.DontLogBodies {
			l.ResponseBody = ""
		}
		slog.Debug("native usage recorded", "type", row.ResourceType, "row", row.ID, "model", t.usage.Model,
			"tier", t.usage.Tier, "requests", t.requests, "cost", rec.Cost)
		p.goAnalyze(func() {
			analytics.RecordExchange(context.Background(), l, rec)
			analyzeBudget(budgetServiceOf(p.gatewayService), llm, app, rec)
		})
	}
}
