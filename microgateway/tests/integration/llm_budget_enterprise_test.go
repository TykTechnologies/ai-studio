//go:build enterprise
// +build enterprise

package integration

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/internal/config"
	mgwdb "github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/microgateway/internal/services"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ap6: an edge refuses an LLM whose own monthly budget is spent, with no
// upstream call, on /llm/ and in the /ai/ bridge's envelope as it refuses a
// spent App budget; an LLM under its budget is served, and App budgets work
// as before.
func TestLLMBudget_EnforcedOnEdge(t *testing.T) {
	const overToken, appBudgetToken = "llm-over-budget-token", "app-over-budget-token"
	var (
		db                *gorm.DB
		under, over, free *mgwdb.LLM
		budgetedApp       *mgwdb.App
	)
	baseURL, backend := setupTrafficTestSeeded(t, config.GatewayConfig{}, func(d *gorm.DB, llm *mgwdb.LLM, app *mgwdb.App) {
		db, under = d, llm
		require.NoError(t, d.Model(llm).Update("monthly_budget", 1.0).Error)
		over = &mgwdb.LLM{Name: "Over Budget", Slug: "over-budget", Vendor: "openai", Endpoint: llm.Endpoint,
			APIKeyEncrypted: llm.APIKeyEncrypted, DefaultModel: "gpt-4", IsActive: true, MonthlyBudget: 0.0005}
		require.NoError(t, d.Create(over).Error)
		require.NoError(t, d.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: over.ID, IsActive: true}).Error)
		// monthly_budget 0 and no llm_blocks entry: no limit on the edge.
		free = &mgwdb.LLM{Name: "No Budget", Slug: "no-budget", Vendor: "openai", Endpoint: llm.Endpoint,
			APIKeyEncrypted: llm.APIKeyEncrypted, DefaultModel: "gpt-4", IsActive: true}
		require.NoError(t, d.Create(free).Error)
		require.NoError(t, d.Create(&mgwdb.AppLLM{AppID: app.ID, LLMID: free.ID, IsActive: true}).Error)
		require.NoError(t, d.Create(&mgwdb.APIToken{Token: overToken, Name: "over", AppID: app.ID, IsActive: true}).Error)
		budgetedApp = &mgwdb.App{Name: "Budgeted", IsActive: true, MonthlyBudget: 0.0005}
		require.NoError(t, d.Create(budgetedApp).Error)
		require.NoError(t, d.Create(&mgwdb.AppLLM{AppID: budgetedApp.ID, LLMID: llm.ID, IsActive: true}).Error)
		require.NoError(t, d.Create(&mgwdb.APIToken{Token: appBudgetToken, Name: "budgeted", AppID: budgetedApp.ID, IsActive: true}).Error)
	})

	// The control plane reports the spend across Studio and every edge.
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, 0).Add(-time.Second)
	spend := func(usage float64) services.AppBudgetData {
		return services.AppBudgetData{Usage: usage, PeriodStart: start, PeriodEnd: end}
	}
	sync := func(seq uint64, p services.BudgetSyncPayload) {
		p.SequenceNumber, p.BlocksIncluded, p.LLMIncluded = seq, true, true
		ev, err := eventbridge.NewEvent(services.BudgetSyncTopic, "control", eventbridge.DirDown, p)
		require.NoError(t, err)
		services.NewBudgetSyncHandler(db).HandleBudgetSync(ev)
	}
	sync(uint64(now.UnixNano()), services.BudgetSyncPayload{
		LLMBudgets: map[uint32]services.AppBudgetData{uint32(over.ID): spend(0.010992), uint32(under.ID): spend(0.0001), uint32(free.ID): spend(50)},
		AppBudgets: map[uint32]services.AppBudgetData{uint32(budgetedApp.ID): spend(0.01)},
	})
	t.Cleanup(func() { sync(uint64(time.Now().UnixNano()), services.BudgetSyncPayload{}) })

	chat := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
	before := backend.GetRequestCount()
	status, body := gatewayPost(t, baseURL+"/llm/rest/over-budget/v1/chat/completions", overToken, chat)
	assert.Equal(t, 403, status, string(body))
	assert.Contains(t, string(body), "Budget limit exceeded")
	status, body = gatewayPost(t, baseURL+"/ai/over-budget/v1/chat/completions", overToken, chat)
	assert.Equal(t, 403, status, string(body))
	assert.Contains(t, string(body), `"budget_exceeded"`)
	assert.Contains(t, string(body), "LLM monthly budget exceeded")
	assert.Contains(t, string(body), "limit=0.0005")
	assert.Equal(t, before, backend.GetRequestCount(), "a refused request never reaches the vendor")

	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", overToken, chat)
	assert.Equal(t, 200, status, "an LLM under its budget is served: %s", body)

	status, body = gatewayPost(t, baseURL+"/ai/no-budget/v1/chat/completions", overToken, chat)
	assert.Equal(t, 200, status, "an LLM with monthly_budget 0 and no llm_blocks entry is not refused: %s", body)

	// A spent App budget is refused as before, on the LLM under its budget.
	before = backend.GetRequestCount()
	status, body = gatewayPost(t, baseURL+"/ai/mock-openai/v1/chat/completions", appBudgetToken, chat)
	assert.Equal(t, 403, status, string(body))
	assert.Contains(t, string(body), `"budget_exceeded"`)
	assert.NotContains(t, string(body), "LLM monthly budget")
	assert.Equal(t, before, backend.GetRequestCount())
}
