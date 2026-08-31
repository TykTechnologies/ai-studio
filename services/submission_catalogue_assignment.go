package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// assignSubmissionCatalogues puts a newly approved resource into the catalogues
// the reviewer chose.
//
// ApproveSubmission accepted assigned_catalogues, stored it on the submission
// and serialised it back -- and nothing ever read it. The field was write-only,
// so a reviewer who supplied catalogues got a resource that was still in none
// of them, and the portal showed nothing until an administrator edited a
// catalogue by hand. Offering a catalogue picker on top of that would have been
// worse than offering none: it would tell the reviewer they had published
// something they had not.
//
// Runs inside the approval transaction, so a failure here rolls the approval
// back rather than leaving a resource stranded.
func (s *Service) assignSubmissionCatalogues(tx *gorm.DB, submission *models.Submission, resourceID uint) error {
	ids := catalogueIDsFrom(submission.AssignedCatalogues)
	if len(ids) == 0 {
		return nil
	}

	switch submission.ResourceType {
	case models.SubmissionResourceTypeTool:
		for _, id := range ids {
			catalogue := &models.ToolCatalogue{ID: id}
			if err := tx.First(catalogue, id).Error; err != nil {
				return fmt.Errorf("tool catalogue %d not found: %w", id, err)
			}
			if err := tx.Model(catalogue).Association("Tools").
				Append(&models.Tool{ID: resourceID}); err != nil {
				return fmt.Errorf("failed to add tool to catalogue %d: %w", id, err)
			}
		}
	case models.SubmissionResourceTypeDatasource:
		for _, id := range ids {
			catalogue := &models.DataCatalogue{ID: id}
			if err := tx.First(catalogue, id).Error; err != nil {
				return fmt.Errorf("data catalogue %d not found: %w", id, err)
			}
			if err := tx.Model(catalogue).Association("Datasources").
				Append(&models.Datasource{ID: resourceID}); err != nil {
				return fmt.Errorf("failed to add datasource to catalogue %d: %w", id, err)
			}
		}
	default:
		return fmt.Errorf("cannot assign catalogues for resource type %q", submission.ResourceType)
	}

	return nil
}

// catalogueIDsFrom normalises the stored catalogue list. It is a JSONMap, so
// after a round trip through JSON the ids arrive as float64 rather than uint,
// and older rows may carry the list under a key rather than as a bare array.
func catalogueIDsFrom(assigned models.JSONMap) []uint {
	if assigned == nil {
		return nil
	}

	var out []uint
	appendID := func(v interface{}) {
		switch n := v.(type) {
		case float64:
			if n > 0 {
				out = append(out, uint(n))
			}
		case int:
			if n > 0 {
				out = append(out, uint(n))
			}
		case uint:
			if n > 0 {
				out = append(out, n)
			}
		}
	}

	for _, v := range assigned {
		switch list := v.(type) {
		case []interface{}:
			for _, item := range list {
				appendID(item)
			}
		default:
			appendID(v)
		}
	}

	return out
}
