package scripting

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting/llmclient"
)

// SetJudgeCallForTesting replaces the LLM call the guardrail judge makes,
// for tests in other packages (hub and edge integration tests), and clears
// the judge's opinion cache. It returns the restore function. Not for
// production use.
func SetJudgeCallForTesting(fn func(ctx context.Context, llm *models.LLM, model, system, user string) (llmclient.Completion, error)) (restore func()) {
	prev := judgeCall
	judgeCall = fn
	judgeCache.Reset()
	return func() {
		judgeCall = prev
		judgeCache.Reset()
	}
}
