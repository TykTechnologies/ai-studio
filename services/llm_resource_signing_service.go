package services

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// RotateLLMResourceSigningSecret gives the LLM a new native API resource
// signing secret; the old key stays verifiable for grace (0 revokes it). It
// emits system.llm.updated, which marks edges pending so the new key reaches
// them with the next configuration push. A concurrent rotation of the same
// LLM fails with models.ErrResourceSigningConflict.
func (s *Service) RotateLLMResourceSigningSecret(id uint, grace time.Duration, userID uint) (*models.LLM, error) {
	if err := models.RotateLLMResourceSigningSecret(s.DB, id, grace, time.Now()); err != nil {
		return nil, err
	}
	llm, err := s.GetLLMByID(id)
	if err != nil {
		return nil, err
	}
	if s.SystemEvents != nil {
		s.SystemEvents.EmitLLMUpdated(llm, llm.ID, userID)
	}
	return llm, nil
}
