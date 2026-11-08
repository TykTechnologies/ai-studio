package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/rs/zerolog/log"
)

// LLMLimitFunc reports the licence's LLM limit (licensing.Service.MaxLLMs).
type LLMLimitFunc func() (max int64, unlimited bool)

// LLMLimitError refuses an LLM create that would take the gateway past its
// licensed LLM limit. The API answers it with 409 QUOTA_EXCEEDED.
type LLMLimitError struct {
	Max int64
}

func (e *LLMLimitError) Error() string {
	noun := "LLMs"
	if e.Max == 1 {
		noun = "LLM"
	}
	return fmt.Sprintf("Your organisation has reached its licensed limit of %d %s. Remove an LLM to make room, or contact your administrator to increase the quota.", e.Max, noun)
}

// IsLLMLimitError reports whether err is an LLM limit refusal.
func IsLLMLimitError(err error) bool {
	var le *LLMLimitError
	return errors.As(err, &le)
}

// SetLLMLimit makes CreateLLM hold this gateway's LLMs to the licence's
// limit. Without it (tests, Community Edition) LLMs are not limited.
func (s *ManagementService) SetLLMLimit(limit LLMLimitFunc) {
	s.llmLimit = limit
}

// countLLMs counts every LLM not deleted, active or not.
func countLLMs(db *gorm.DB) (int64, error) {
	var n int64
	err := db.Model(&database.LLM{}).Count(&n).Error
	return n, err
}

// createLLMWithinLimit stores llm unless that would take the LLM count past
// the licensed limit. Only increases are refused: after a licence downgrade
// the existing LLMs stay. The count and the insert are serialised in this
// process; a gateway's own management API is the only writer of its LLMs.
func (s *ManagementService) createLLMWithinLimit(llm *database.LLM) error {
	if s.llmLimit == nil {
		return s.repo.CreateLLM(llm)
	}
	max, unlimited := s.llmLimit()
	if unlimited {
		return s.repo.CreateLLM(llm)
	}
	s.llmLimitMu.Lock()
	defer s.llmLimitMu.Unlock()
	n, err := countLLMs(s.db)
	if err != nil {
		return fmt.Errorf("failed to count LLMs: %w", err)
	}
	if n >= max {
		refusal := &LLMLimitError{Max: max}
		log.Warn().Int64("llms", n).Int64("max_llms", max).Msg("LLM not created: " + refusal.Error())
		return refusal
	}
	return s.repo.CreateLLM(llm)
}

// WarnIfAtLLMLimit logs a warning when the gateway's LLMs are at or over the
// licensed limit, so an operator sees it at start-up.
func WarnIfAtLLMLimit(db *gorm.DB, limit LLMLimitFunc) {
	if limit == nil {
		return
	}
	max, unlimited := limit()
	if unlimited {
		return
	}
	n, err := countLLMs(db)
	if err != nil {
		log.Warn().Err(err).Msg("Could not count LLMs against the licensed limit")
		return
	}
	switch {
	case n > max:
		log.Warn().Int64("llms", n).Int64("max_llms", max).Msg("This gateway has more LLMs than its licence allows; no more can be created. Remove LLMs or contact Tyk to increase the limit.")
	case n == max:
		log.Warn().Int64("llms", n).Int64("max_llms", max).Msg("This gateway has reached its licensed LLM limit; no more can be created. Remove an LLM or contact Tyk to increase the limit.")
	}
}
