package services

import (
	"errors"
	"fmt"
	"hash/fnv"

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

// llmLimitLockKey is the Postgres advisory lock that serialises LLM creates
// under the licence limit across gateways sharing a database.
var llmLimitLockKey = func() int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("microgateway|llm-limit"))
	return int64(h.Sum64())
}()

// lockLLMLimit serialises LLM creates under the limit for the rest of tx: a
// transaction-scoped advisory lock on Postgres. SQLite needs none, since the
// gateway opens it with txlock=immediate, so a transaction holds the write
// lock from its start.
func lockLLMLimit(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", llmLimitLockKey).Error
}

// createLLMWithinLimit stores llm unless that would take the LLM count past
// the licensed limit. Only increases are refused: after a licence downgrade
// the existing LLMs stay. The count and the insert run in one transaction
// under lockLLMLimit, so gateways sharing a database cannot both pass the
// check.
func (s *ManagementService) createLLMWithinLimit(llm *database.LLM) error {
	if s.llmLimit == nil {
		return s.repo.CreateLLM(llm)
	}
	max, unlimited := s.llmLimit()
	if unlimited {
		return s.repo.CreateLLM(llm)
	}
	var n int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := lockLLMLimit(tx); err != nil {
			return fmt.Errorf("failed to lock the LLM limit: %w", err)
		}
		var err error
		if n, err = countLLMs(tx); err != nil {
			return fmt.Errorf("failed to count LLMs: %w", err)
		}
		if n >= max {
			return &LLMLimitError{Max: max}
		}
		return database.NewRepository(tx).CreateLLM(llm)
	})
	if IsLLMLimitError(err) {
		log.Warn().Int64("llms", n).Int64("max_llms", max).Msg("LLM not created: " + err.Error())
	}
	return err
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
