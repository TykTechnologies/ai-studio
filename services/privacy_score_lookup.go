package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// privacyResource is the only part of a resource the privacy check reads: its
// name, for the refusal message, and its score.
type privacyResource struct {
	Name  string
	Score int
}

// The privacy check previously fetched every LLM, datasource and tool one row
// at a time inside its loops -- len(llmIDs)+len(datasourceIDs)+len(toolIDs)
// queries on every app create and update. These load each kind in a single
// query instead, so the check costs at most three.
//
// Two behaviours the per-ID version had, which these keep:
//
//   - a requested id that does not exist is an error, not a silently missing
//     row. Find() returns fewer rows rather than failing, so the count is
//     checked and the offending id named.
//   - callers look results up in the order they supplied, so the resource
//     reported in a refusal is the same one as before.
//
// Selecting only the three needed columns also avoids the per-row secret
// resolution GetLLMByID does, which this check never uses.

func (s *Service) loadLLMPrivacyScores(ids []uint) (map[uint]privacyResource, error) {
	out := make(map[uint]privacyResource, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []models.LLM
	if err := s.DB.Select("id", "name", "privacy_score").
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = privacyResource{Name: r.Name, Score: r.PrivacyScore}
	}
	return out, assertAllFound(out, ids, "LLM")
}

func (s *Service) loadDatasourcePrivacyScores(ids []uint) (map[uint]privacyResource, error) {
	out := make(map[uint]privacyResource, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []models.Datasource
	if err := s.DB.Select("id", "name", "privacy_score").
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = privacyResource{Name: r.Name, Score: r.PrivacyScore}
	}
	return out, assertAllFound(out, ids, "datasource")
}

func (s *Service) loadToolPrivacyScores(ids []uint) (map[uint]privacyResource, error) {
	out := make(map[uint]privacyResource, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	var rows []models.Tool
	if err := s.DB.Select("id", "name", "privacy_score").
		Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ID] = privacyResource{Name: r.Name, Score: r.PrivacyScore}
	}
	return out, assertAllFound(out, ids, "tool")
}

// assertAllFound reports the first requested id that came back missing, in the
// order the caller supplied, so the error is deterministic.
func assertAllFound(found map[uint]privacyResource, ids []uint, kind string) error {
	for _, id := range ids {
		if _, ok := found[id]; !ok {
			return fmt.Errorf("%s %d not found", kind, id)
		}
	}
	return nil
}
