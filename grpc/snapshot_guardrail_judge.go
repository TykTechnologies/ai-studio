package grpc

import (
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// edgeJudgeConfig is a guardrail filter's config as edges receive it: without
// its judge when the judge LLM's privacy score is below that of an LLM the
// filter guards in this snapshot (llmIDs). Edges do not receive LLM privacy
// scores, so this is where the rule (flagged text never goes to a provider
// scored below its source) is kept for them; the hub checks it at save and
// again at run time. Without the judge an edge keeps the filter's own
// verdict. config itself is never modified.
func edgeJudgeConfig(filterID uint, config models.JSONMap, llmIDs []uint32, privacy map[uint]int) models.JSONMap {
	if config == nil {
		return config
	}
	cfg, err := guardrails.ParseConfig(config)
	if err != nil || !cfg.Judge.Enabled() {
		return config
	}
	judgeScore, known := privacy[cfg.Judge.LLMID]
	if !known {
		// Not in this snapshot: the edge cannot find it and gives no opinion.
		return config
	}
	for _, id := range llmIDs {
		if score, ok := privacy[uint(id)]; ok && judgeScore < score {
			logger.Log.Warn().Uint("filter_id", filterID).Uint("judge_llm_id", cfg.Judge.LLMID).
				Msg("Guardrail judge scored below an LLM the filter guards - sent to edges without its judge")
			out := make(models.JSONMap, len(config))
			for k, v := range config {
				out[k] = v
			}
			delete(out, "judge")
			return out
		}
	}
	return config
}
