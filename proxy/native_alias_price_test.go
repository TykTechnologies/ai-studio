package proxy

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/budget"
	anthropicVendor "github.com/TykTechnologies/midsommar/v2/vendors/anthropic"
	openaiVendor "github.com/TykTechnologies/midsommar/v2/vendors/openai"
)

// Usage accounted after the request has gone (a batch's results, a background
// response) is priced like a synchronous call: by the model the vendor
// reported and, when that has no price, by the model the client asked for.
// The results only name the vendor's dated snapshot, so the ledger row keeps
// the requested models. Before, the reported name was passed as the requested
// one and a batch on a model priced under its alias was recorded at zero cost
// and never reached a budget (midsommar-clean-6bg5.6).

const (
	aliasModel = "claude-haiku-4-5"
	datedModel = "claude-haiku-4-5-20251001"
)

// An Anthropic batch created through the gateway on an alias-priced model:
// the row keeps the alias, and the results are priced under it at the batch
// multiplier and charged to the budget.
func TestNativeAliasPrice_AnthropicBatchCreatedOnAliasIsPricedAndCharged(t *testing.T) {
	t.Parallel()
	spend := &spendCounter{}
	h := newNativeHarness(t, nativeHarnessOpts{wrapBudget: func(s budget.Service) budget.Service { spend.Service = s; return spend }})
	h.addPrice(aliasModel, 0.00001, 0.000002)

	resp, out := h.do(h.keyA, http.MethodPost, batchesPath, batchCreateBody(aliasModel, aliasModel), "anthropic-version", "2023-06-01")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))
	id := decodeBatch(t, out)["id"].(string)
	row, err := h.service.NativeResourceStore().GetByOwner(context.Background(), h.llm.ID, h.appA.ID, anthropicVendor.ResourceMessageBatch, "msgbatch_N1")
	require.NoError(t, err)
	assert.Equal(t, []string{aliasModel}, row.RequestedModelList(), "the row keeps what the batch's requests asked for")
	assert.NotContains(t, string(out), "requested_models", "the list is not returned to the client")

	h.vendor.addBatch("msgbatch_N1", "ended")
	h.vendor.setResults("msgbatch_N1", succeededLine("req-a", datedModel, 18, 8), succeededLine("req-b", datedModel, 2, 2))
	resp, out = h.do(h.keyA, http.MethodGet, resultsPath(id), "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

	var recs []models.LLMChatRecord
	require.Eventually(t, func() bool {
		recs = h.chatRecords(h.appA)
		return len(recs) == 1
	}, 5*time.Second, 50*time.Millisecond)
	rec := recs[0]
	assert.Equal(t, datedModel, rec.Name, "name stays the vendor-reported model")
	assert.Equal(t, aliasModel, rec.RequestedModel, "requested_model is the alias the client sent")
	assert.Equal(t, 20, rec.PromptTokens)
	assert.Equal(t, 10, rec.ResponseTokens)
	assert.Equal(t, "batch", rec.ServiceTier)
	want := (10*0.00001 + 20*0.000002) * 0.5 * 10000
	assert.InDelta(t, want, rec.Cost, 1e-9, "the alias price at the batch multiplier")

	total, calls := spend.totals()
	assert.Equal(t, 1, calls)
	assert.InDelta(t, want/10000, total, 1e-12, "the budget is charged the same amount")
}

// Which price applies: the reported model's when it has one (as for a
// synchronous call), else the requested alias's; with neither the record is
// still written, at zero cost.
func TestNativeAliasPrice_Precedence(t *testing.T) {
	t.Parallel()
	const (
		aliasCPT, aliasCPIT = 0.00001, 0.000002
		datedCPT, datedCPIT = 0.00004, 0.000008
	)
	for _, tc := range []struct {
		name         string
		alias, dated bool
		requested    []string
		wantCPT      float64
		wantCPIT     float64
		wantReq      string
	}{
		{name: "price under the alias only", alias: true, requested: []string{aliasModel}, wantCPT: aliasCPT, wantCPIT: aliasCPIT, wantReq: aliasModel},
		{name: "price under the dated name only", dated: true, requested: []string{aliasModel}, wantCPT: datedCPT, wantCPIT: datedCPIT, wantReq: aliasModel},
		{name: "both: the reported (dated) model's price wins", alias: true, dated: true, requested: []string{aliasModel}, wantCPT: datedCPT, wantCPIT: datedCPIT, wantReq: aliasModel},
		{name: "no price under either name: recorded at zero", requested: []string{aliasModel}, wantReq: aliasModel},
		{name: "row from before the column: priced by the reported name only", alias: true, wantReq: datedModel},
		{name: "alias among several requested models", alias: true, requested: []string{"claude-sol", aliasModel}, wantCPT: aliasCPT, wantCPIT: aliasCPIT, wantReq: aliasModel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newNativeHarness(t, nativeHarnessOpts{})
			if tc.alias {
				h.addPrice(aliasModel, aliasCPT, aliasCPIT)
			}
			if tc.dated {
				h.addPrice(datedModel, datedCPT, datedCPIT)
			}
			h.vendor.addBatch("msgbatch_P1", "ended")
			h.vendor.setResults("msgbatch_P1", succeededLine("a", datedModel, 1000, 200))
			row := h.seedBatchRow(h.llm, h.appA, "msgbatch_P1")
			if tc.requested != nil {
				row.SetRequestedModels(tc.requested)
				require.NoError(t, h.db.Model(&models.NativeResource{}).Where("id = ?", row.ID).
					Update("requested_models", row.RequestedModels).Error)
			}

			resp, out := h.do(h.keyA, http.MethodGet, resultsPath(h.wrap(h.llm, h.appA, "msgbatch_P1")), "")
			require.Equal(t, http.StatusOK, resp.StatusCode, string(out))

			var recs []models.LLMChatRecord
			require.Eventually(t, func() bool {
				recs = h.chatRecords(h.appA)
				return len(recs) == 1
			}, 5*time.Second, 50*time.Millisecond)
			assert.Equal(t, datedModel, recs[0].Name)
			assert.Equal(t, tc.wantReq, recs[0].RequestedModel)
			assert.Equal(t, 1000, recs[0].PromptTokens)
			assert.Equal(t, 200, recs[0].ResponseTokens)
			assert.InDelta(t, (200*tc.wantCPT+1000*tc.wantCPIT)*0.5*10000, recs[0].Cost, 1e-9)
		})
	}
}

// OpenAI batches: the requested models are admitted with the input file, so
// the batch's row inherits them from the file's, and a price set under the
// requested name applies when the output names a snapshot without one.
func TestNativeAliasPrice_OpenAIBatchInheritsRequestedModelsFromItsInputFile(t *testing.T) {
	t.Parallel()
	h := newBatchesHarness(t)
	ctx := context.Background()
	store := h.service.NativeResourceStore()
	// The harness prices the reported snapshot (gpt-4o-2026); price the
	// requested name instead.
	require.NoError(t, h.db.Where("model_name = ?", "gpt-4o-2026").Delete(&models.ModelPrice{}).Error)
	require.NoError(t, h.db.Create(&models.ModelPrice{ModelName: "gpt-4o", Vendor: string(models.OPENAI),
		CPT: 2, CPIT: 1, CacheReadPT: 0.5, Currency: "USD"}).Error)
	require.NoError(t, h.proxy.loadResources())

	resp, out := h.uploadFile(h.keyC, chatLine("r1", "gpt-4o", "hello"), chatLine("r2", "gpt-4o", "there"))
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	file := decodeObj(t, out)["id"].(string)
	fileRow, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceFile, vendorPart(file))
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-4o"}, fileRow.RequestedModelList())

	resp, out = h.do(h.keyC, http.MethodPost, "/llm/call/primary/v1/batches",
		`{"input_file_id":"`+file+`","endpoint":"/v1/chat/completions","completion_window":"24h"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	batch := decodeObj(t, out)["id"].(string)
	row, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceBatch, vendorPart(batch))
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-4o"}, row.RequestedModelList(), "inherited from the input file's row")

	h.api.completeBatch(vendorPart(batch), outputLine("r1", 100, 20, 40), outputLine("r2", 10, 5, 0))
	resp, out = h.do(h.keyC, http.MethodGet, "/llm/call/primary/v1/batches/"+batch, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", out)
	h.proxy.reconcileNative(ctx, store, time.Now())
	h.proxy.reconcileNative(ctx, store, time.Now().Add(time.Hour))

	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 1)
	assert.Equal(t, "gpt-4o-2026", recs[0].Name)
	assert.Equal(t, "gpt-4o", recs[0].RequestedModel)
	assert.InDelta(t, 0.5*models.ChatRecordCost(string(models.OPENAI), 2, 1, 0, 0.5, 70, 25, 0, 40), recs[0].Cost, 1e-9,
		"the requested model's price at the batch multiplier")
}

// A background response is accounted by the reconciler from the polled
// object, which names the snapshot: the row keeps the request's model.
func TestNativeAliasPrice_BackgroundResponsePricedUnderRequestedModel(t *testing.T) {
	t.Parallel()
	h := newResponsesHarness(t)
	ctx := context.Background()
	store := h.service.NativeResourceStore()
	require.NoError(t, h.db.Where("model_name = ?", "gpt-4o-2026").Delete(&models.ModelPrice{}).Error)
	require.NoError(t, h.db.Create(&models.ModelPrice{ModelName: "gpt-4o", Vendor: string(models.OPENAI),
		CPT: 2, CPIT: 1, CacheReadPT: 0.5, Currency: "USD"}).Error)
	require.NoError(t, h.proxy.loadResources())

	resp, body := h.do(h.keyC, http.MethodPost, "/llm/rest/primary/v1/responses", `{"model":"gpt-4o","input":"hi","background":true}`)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", body)
	row, err := store.GetByOwner(ctx, h.primary.ID, h.appC.ID, openaiVendor.ResourceResponse, "resp_1")
	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-4o"}, row.RequestedModelList())

	h.api.complete("resp_1")
	h.proxy.reconcileNative(ctx, store, time.Now())
	recs := h.chatRecords(h.appC)
	require.Len(t, recs, 1)
	assert.Equal(t, "gpt-4o-2026", recs[0].Name)
	assert.Equal(t, "gpt-4o", recs[0].RequestedModel)
	assert.InDelta(t, models.ChatRecordCost(string(models.OPENAI), 2, 1, 0, 0.5, 8, 5, 0, 2), recs[0].Cost, 1e-9)
}
