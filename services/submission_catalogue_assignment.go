package services

import (
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
//
// The catalogues are loaded in one query and the resource appended to all of
// them in one statement. Fetching and appending per id cost 2N queries inside
// the approval transaction, which holds locks for as long as it runs.
func (s *Service) assignSubmissionCatalogues(tx *gorm.DB, submission *models.Submission, resourceID uint) error {
	ids := catalogueIDsFrom(submission.AssignedCatalogues)
	if len(ids) == 0 {
		return nil
	}

	switch submission.ResourceType {
	case models.SubmissionResourceTypeTool:
		var catalogues []models.ToolCatalogue
		if err := tx.Where("id IN ?", ids).Find(&catalogues).Error; err != nil {
			return fmt.Errorf("failed to load tool catalogues: %w", err)
		}
		found := make([]uint, len(catalogues))
		for i, c := range catalogues {
			found[i] = c.ID
		}
		if err := assertCataloguesFound(ids, found, "tool catalogue"); err != nil {
			return err
		}
		if err := appendMemberships(tx, "tool_catalogue_tools", "tool_catalogue_id", "tool_id", ids, resourceID); err != nil {
			return fmt.Errorf("failed to add tool to catalogues: %w", err)
		}
	case models.SubmissionResourceTypeDatasource:
		var catalogues []models.DataCatalogue
		if err := tx.Where("id IN ?", ids).Find(&catalogues).Error; err != nil {
			return fmt.Errorf("failed to load data catalogues: %w", err)
		}
		found := make([]uint, len(catalogues))
		for i, c := range catalogues {
			found[i] = c.ID
		}
		if err := assertCataloguesFound(ids, found, "data catalogue"); err != nil {
			return err
		}
		if err := appendMemberships(tx, "data_catalogue_data_sources", "data_catalogue_id", "datasource_id", ids, resourceID); err != nil {
			return fmt.Errorf("failed to add datasource to catalogues: %w", err)
		}
	default:
		return fmt.Errorf("cannot assign catalogues for resource type %q", submission.ResourceType)
	}

	return nil
}

// appendMemberships writes the join rows for every chosen catalogue in one
// statement.
//
// The obvious spelling, tx.Model(&catalogues).Association(...).Append(...),
// is worse than the per-id loop it replaces: association mode pairs values
// with rows positionally, so it needs the resource repeated once per
// catalogue, and it then issues an upsert of the resource row and an
// updated_at touch of the catalogue for each one -- 3N+1 statements, and a
// write to a resource this function has no business modifying. The join table
// is the only thing that actually needs a row, so it is written directly, the
// way ensureToolInDefaultCatalogueTx already reads it.
//
// ON CONFLICT DO NOTHING keeps a re-approval idempotent, which is what
// association append gave us before.
func appendMemberships(tx *gorm.DB, table, catalogueColumn, resourceColumn string, catalogueIDs []uint, resourceID uint) error {
	rows := make([]map[string]interface{}, 0, len(catalogueIDs))
	for _, id := range catalogueIDs {
		rows = append(rows, map[string]interface{}{
			catalogueColumn: id,
			resourceColumn:  resourceID,
		})
	}

	return tx.Table(table).Clauses(clause.OnConflict{DoNothing: true}).Create(rows).Error
}

// assertCataloguesFound reports a requested catalogue that does not exist.
//
// Find() returns the rows it has rather than failing on a missing one, so
// without this a reviewer naming a catalogue id that no longer exists would
// have the resource quietly published to the others and the bad id ignored --
// the per-id version this replaced returned an error, and so does this. The
// requested order is walked rather than the returned order so the id named is
// the same one the caller would have hit first.
func assertCataloguesFound(requested, found []uint, kind string) error {
	if len(found) == len(requested) {
		return nil
	}

	present := make(map[uint]struct{}, len(found))
	for _, id := range found {
		present[id] = struct{}{}
	}
	for _, id := range requested {
		if _, ok := present[id]; !ok {
			return fmt.Errorf("%s %d not found", kind, id)
		}
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

// ensureToolInDefaultCatalogueTx puts a tool into the Default tool catalogue
// when it is not in any catalogue yet.
//
// This mirrors ensureDatasourceInDefaultCatalogue. Before it, approving a tool
// submission created a real Tool in NO catalogue at all, while a hand-created
// data source auto-joined Default -- so the platform published the sensitive
// thing automatically and withheld the reviewed one. Both now publish to
// Default, and both arrive inactive so publication is not the same as going
// live.
//
// Default membership is a Community/Enterprise compatibility rule: CE has no
// catalogue management, so anything outside Default has no CE equivalent.
func (s *Service) ensureToolInDefaultCatalogueTx(tx *gorm.DB, tool *models.Tool) error {
	// Tool has no back-reference to ToolCatalogue -- the relation is declared
	// only on ToolCatalogue.Tools -- so count rows in the join table directly.
	var count int64
	if err := tx.Table("tool_catalogue_tools").
		Where("tool_id = ?", tool.ID).Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check tool catalogue membership: %w", err)
	}
	if count > 0 {
		return nil
	}

	defaultCatalogue, err := models.GetOrCreateDefaultToolCatalogue(tx)
	if err != nil {
		return fmt.Errorf("failed to get default tool catalogue: %w", err)
	}

	if err := tx.Model(defaultCatalogue).Association("Tools").Append(tool); err != nil {
		return fmt.Errorf("failed to add tool to default catalogue: %w", err)
	}

	return nil
}
