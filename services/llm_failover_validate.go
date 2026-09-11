package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/modelmatch"
)

// LLMFailoverValidationError names the rung of the waterfall that failed
// validation so the API can answer 400 and the form can highlight the row.
// Index is -1 when the problem is not tied to one target.
type LLMFailoverValidationError struct {
	Index  int
	Field  string
	Detail string
}

func (e *LLMFailoverValidationError) Error() string {
	if e.Index < 0 {
		return "failover: " + e.Detail
	}
	return fmt.Sprintf("failover target %d: %s", e.Index+1, e.Detail)
}

func failoverErr(index int, field, format string, args ...interface{}) error {
	return &LLMFailoverValidationError{Index: index, Field: field, Detail: fmt.Sprintf(format, args...)}
}

// ValidateLLMFailover checks a waterfall against the LLM it is attached to.
// primary.ID is zero on create; every other primary field is already final.
//
// The rules exist because a fallback silently widens what the app can reach:
// access to a fallback is inherited from the primary, so the fallback must be
// at least as private, it must live where the primary lives (same namespace
// or global, or an edge in that namespace would not have it), and the model
// must be one the fallback actually allows -- using the same matcher the
// proxy applies at request time.
func (s *Service) ValidateLLMFailover(primary *models.LLM, f models.LLMFailover) error {
	if !f.Enabled() {
		if f.Triggers != nil {
			return failoverErr(-1, "targets", "triggers were given but there are no failover targets")
		}
		return nil
	}
	if len(f.Targets) > models.MaxFailoverTargets {
		return failoverErr(-1, "targets", "at most %d failover targets are allowed", models.MaxFailoverTargets)
	}

	seen := map[models.LLMFailoverTarget]bool{}
	for i, t := range f.Targets {
		if t.LLMID == 0 {
			return failoverErr(i, "llm_id", "an LLM must be selected")
		}
		if t.Model == "" {
			return failoverErr(i, "model", "a model is required")
		}
		if primary.ID != 0 && t.LLMID == primary.ID {
			return failoverErr(i, "llm_id", "an LLM cannot fail over to itself")
		}
		if seen[t] {
			return failoverErr(i, "llm_id", "duplicate failover target (LLM %d, model %q)", t.LLMID, t.Model)
		}
		seen[t] = true

		target, err := s.GetLLMByID(t.LLMID)
		if err != nil {
			return failoverErr(i, "llm_id", "LLM %d does not exist", t.LLMID)
		}
		if !target.Active {
			return failoverErr(i, "llm_id", "LLM %q is not active", target.Name)
		}
		ok, err := modelmatch.AllowedStrict(target.AllowedModels, t.Model)
		if err != nil {
			return failoverErr(i, "model", "LLM %q has an %v", target.Name, err)
		}
		if !ok {
			return failoverErr(i, "model", "model %q is not in the allowed models of LLM %q", t.Model, target.Name)
		}
		if target.PrivacyScore < primary.PrivacyScore {
			return failoverErr(i, "llm_id", "LLM %q has a lower privacy score (%d) than this LLM (%d)", target.Name, target.PrivacyScore, primary.PrivacyScore)
		}
		if target.Namespace != "" && target.Namespace != primary.Namespace {
			return failoverErr(i, "llm_id", "LLM %q is in namespace %q, which this LLM (namespace %q) cannot reach", target.Name, target.Namespace, primary.Namespace)
		}
	}

	if f.Triggers != nil {
		for _, code := range f.Triggers.StatusCodes {
			if !models.FailoverStatusCodeAllowed(code) {
				return failoverErr(-1, "triggers.status_codes", "status %d cannot trigger failover; only 5xx, 408 and 429 can", code)
			}
		}
		if f.Triggers.AttemptTimeoutSecond < 0 {
			return failoverErr(-1, "triggers.attempt_timeout_seconds", "must not be negative")
		}
	}
	return nil
}

// LLMsReferencingAsFailoverTarget returns the names of LLMs whose waterfall
// points at the given LLM. Used to refuse a delete that would leave dangling
// rungs; the table is small, so this scans in Go rather than in JSON SQL that
// differs between SQLite and Postgres.
func (s *Service) LLMsReferencingAsFailoverTarget(id uint) ([]string, error) {
	var llms []models.LLM
	if err := s.DB.Where("failover IS NOT NULL").Find(&llms).Error; err != nil {
		return nil, err
	}
	var names []string
	for _, llm := range llms {
		if llm.ID == id {
			continue
		}
		for _, t := range llm.Failover.Targets {
			if t.LLMID == id {
				names = append(names, llm.Name)
				break
			}
		}
	}
	return names, nil
}
