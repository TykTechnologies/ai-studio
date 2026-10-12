package services

// Shared expectations of the edge batch-accounting tests
// (analytics_handler_native_batch_test.go, native_reconciler_edge_test.go):
// one succeeded batch result on claude-luna, priced at the batch rate.
const (
	batchPromptTokens   = 1500
	batchResponseTokens = 300
	// Per-token list prices of claude-luna: $2 per million input tokens
	// (ModelPrice.CPIT) and $10 per million output tokens (ModelPrice.CPT).
	batchPromptTokenPrice   = 0.000002
	batchResponseTokenPrice = 0.00001
	// batchPriceMultiplier is the Message Batches discount: half the list
	// price.
	batchPriceMultiplier = 0.5
	// costUnitsPerCurrencyUnit: the gateway stores cost in 1/10000 of the
	// currency.
	costUnitsPerCurrencyUnit = 10000

	// batchCost is the expected record and budget cost, in cost units.
	batchCost = (batchPromptTokens*batchPromptTokenPrice + batchResponseTokens*batchResponseTokenPrice) *
		batchPriceMultiplier * costUnitsPerCurrencyUnit
)
