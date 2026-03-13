package openfga

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/authz"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// syncBatchSize controls how many rows are fetched per database query during sync.
const syncBatchSize = 1000

// FullSync reads the database in batches and populates the authorization store.
// Each collector streams rows using keyset pagination and grants relationships
// incrementally, keeping peak memory usage constant regardless of database size.
func (s *Store) FullSync(ctx context.Context, db *gorm.DB) error {
	log.Info().Msg("authz: starting full relationship sync from database")

	type collector struct {
		name string
		fn   func(ctx context.Context, db *gorm.DB, grant grantFunc) error
	}

	collectors := []collector{
		{"system", syncSystemRels},
		{"group members", syncGroupMemberRels},
		{"catalogues", syncJoinTable("group_catalogues", "group_id", "catalogue_id", "assigned_group", authz.SubjectGroup, catalogueResource)},
		{"data catalogues", syncJoinTable("group_datacatalogues", "group_id", "data_catalogue_id", "assigned_group", authz.SubjectGroup, dataCatalogueResource)},
		{"tool catalogues", syncJoinTable("group_toolcatalogues", "group_id", "tool_catalogue_id", "assigned_group", authz.SubjectGroup, toolCatalogueResource)},
		{"llms", syncJoinTable("catalogue_llms", "catalogue_id", "llm_id", "parent_catalogue", catalogueResource, llmResource)},
		{"datasources", syncDatasourceRels},
		{"tools", syncJoinTable("tool_catalogue_tools", "tool_catalogue_id", "tool_id", "parent_catalogue", toolCatalogueResource, toolResource)},
		{"apps", syncOwnerTable("apps")},
		{"chats", syncJoinTable("chat_groups", "group_id", "chat_id", "assigned_group", authz.SubjectGroup, chatResource)},
		{"plugin resources", syncPluginResourceRels},
		{"submissions", syncSubmissionRels},
	}

	var total int
	grant := func(ctx context.Context, rels []authz.Relationship) error {
		if len(rels) == 0 {
			return nil
		}
		total += len(rels)
		return s.Grant(ctx, rels)
	}

	for _, c := range collectors {
		if err := c.fn(ctx, db, grant); err != nil {
			return fmt.Errorf("authz: sync %s: %w", c.name, err)
		}
	}

	if total == 0 {
		log.Info().Msg("authz: no relationships to sync (empty database)")
	} else {
		log.Info().Int("count", total).Msg("authz: full sync complete")
	}
	return nil
}

// grantFunc writes a batch of relationships to the authorization store.
type grantFunc func(ctx context.Context, rels []authz.Relationship) error

// Resource ID formatters for join table sync.
func catalogueResource(id uint) string     { return authz.ResourceID("catalogue", id) }
func dataCatalogueResource(id uint) string { return authz.ResourceID("data_catalogue", id) }
func toolCatalogueResource(id uint) string { return authz.ResourceID("tool_catalogue", id) }
func llmResource(id uint) string           { return authz.ResourceID("llm", id) }
func toolResource(id uint) string          { return authz.ResourceID("tool", id) }
func chatResource(id uint) string          { return authz.ResourceID("chat", id) }

// syncSystemRels syncs all user system-level roles in a single query with batched reads.
func syncSystemRels(ctx context.Context, db *gorm.DB, grant grantFunc) error {
	type userRow struct {
		ID                uint
		IsAdmin           bool
		AccessToSsoConfig bool
	}

	var lastID uint
	for {
		var rows []userRow
		if err := db.Table("users").
			Select("id, is_admin, access_to_sso_config").
			Where("id > ? AND deleted_at IS NULL", lastID).
			Order("id").
			Limit(syncBatchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		var rels []authz.Relationship
		for _, r := range rows {
			rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(r.ID), Relation: "member", Resource: "system:1"})
			if r.IsAdmin {
				rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(r.ID), Relation: "admin", Resource: "system:1"})
			}
			if r.AccessToSsoConfig {
				rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(r.ID), Relation: "sso_admin", Resource: "system:1"})
			}
			lastID = r.ID
		}
		if err := grant(ctx, rels); err != nil {
			return err
		}
	}
}

// joinRow is a generic struct for scanning join table rows.
type joinRow struct {
	LeftID  uint
	RightID uint
}

// syncGroupMemberRels syncs user-group memberships in batches using ROWID-based keyset pagination.
func syncGroupMemberRels(ctx context.Context, db *gorm.DB, grant grantFunc) error {
	var offset int
	for {
		var rows []joinRow
		if err := db.Table("user_groups").
			Select("group_id as left_id, user_id as right_id").
			Offset(offset).
			Limit(syncBatchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		rels := make([]authz.Relationship, len(rows))
		for i, r := range rows {
			rels[i] = authz.Relationship{
				Subject:  authz.SubjectUser(r.RightID),
				Relation: "member",
				Resource: authz.SubjectGroup(r.LeftID),
			}
		}
		if err := grant(ctx, rels); err != nil {
			return err
		}
		offset += len(rows)
	}
}

// syncJoinTable returns a collector that syncs a two-column join table in batches.
func syncJoinTable(table, leftCol, rightCol, relation string, subjectFn, resourceFn func(uint) string) func(context.Context, *gorm.DB, grantFunc) error {
	return func(ctx context.Context, db *gorm.DB, grant grantFunc) error {
		var offset int
		for {
			var rows []joinRow
			if err := db.Table(table).
				Select(leftCol + " as left_id, " + rightCol + " as right_id").
				Offset(offset).
				Limit(syncBatchSize).
				Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}

			rels := make([]authz.Relationship, len(rows))
			for i, r := range rows {
				rels[i] = authz.Relationship{
					Subject:  subjectFn(r.LeftID),
					Relation: relation,
					Resource: resourceFn(r.RightID),
				}
			}
			if err := grant(ctx, rels); err != nil {
				return err
			}
			offset += len(rows)
		}
	}
}

// syncDatasourceRels syncs data_catalogue->datasource mappings and datasource ownership.
func syncDatasourceRels(ctx context.Context, db *gorm.DB, grant grantFunc) error {
	// Catalogue-to-datasource join.
	syncCatDS := syncJoinTable("data_catalogue_data_sources", "data_catalogue_id", "datasource_id", "parent_catalogue", dataCatalogueResource, func(id uint) string {
		return authz.ResourceID("datasource", id)
	})
	if err := syncCatDS(ctx, db, grant); err != nil {
		return err
	}

	// Datasource ownership.
	return syncOwnerTableAs("datasources", "datasource")(ctx, db, grant)
}

// syncOwnerTable returns a collector for tables with id+user_id ownership (defaults resource type to table name minus trailing "s").
func syncOwnerTable(table string) func(context.Context, *gorm.DB, grantFunc) error {
	resourceType := table[:len(table)-1] // "apps" -> "app"
	return syncOwnerTableAs(table, resourceType)
}

func syncOwnerTableAs(table, resourceType string) func(context.Context, *gorm.DB, grantFunc) error {
	return func(ctx context.Context, db *gorm.DB, grant grantFunc) error {
		type ownerRow struct {
			ID     uint
			UserID uint
		}
		var lastID uint
		for {
			var rows []ownerRow
			if err := db.Table(table).
				Select("id, user_id").
				Where("id > ? AND user_id > 0 AND deleted_at IS NULL", lastID).
				Order("id").
				Limit(syncBatchSize).
				Find(&rows).Error; err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}

			rels := make([]authz.Relationship, len(rows))
			for i, o := range rows {
				rels[i] = authz.Relationship{
					Subject:  authz.SubjectUser(o.UserID),
					Relation: "owner",
					Resource: authz.ResourceID(resourceType, o.ID),
				}
				lastID = o.ID
			}
			if err := grant(ctx, rels); err != nil {
				return err
			}
		}
	}
}

// syncPluginResourceRels syncs group-to-plugin-resource assignments in batches.
func syncPluginResourceRels(ctx context.Context, db *gorm.DB, grant grantFunc) error {
	type pluginResRow struct {
		ID                   uint
		GroupID              uint
		PluginResourceTypeID uint
		InstanceID           string
	}
	var lastID uint
	for {
		var rows []pluginResRow
		if err := db.Table("group_plugin_resources").
			Select("id, group_id, plugin_resource_type_id, instance_id").
			Where("id > ? AND deleted_at IS NULL", lastID).
			Order("id").
			Limit(syncBatchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		var rels []authz.Relationship
		for _, r := range rows {
			if err := authz.ValidateID(r.InstanceID); err != nil {
				log.Warn().Err(err).
					Uint("group_id", r.GroupID).
					Uint("plugin_resource_type_id", r.PluginResourceTypeID).
					Str("instance_id", r.InstanceID).
					Msg("authz: skipping plugin resource with invalid instance ID")
				lastID = r.ID
				continue
			}
			objectID := fmt.Sprintf("%d_%s", r.PluginResourceTypeID, r.InstanceID)
			rels = append(rels, authz.Relationship{
				Subject:  authz.SubjectGroup(r.GroupID),
				Relation: "assigned_group",
				Resource: "plugin_resource:" + objectID,
			})
			lastID = r.ID
		}
		if err := grant(ctx, rels); err != nil {
			return err
		}
	}
}

// syncSubmissionRels syncs submitter and reviewer relationships in batches.
func syncSubmissionRels(ctx context.Context, db *gorm.DB, grant grantFunc) error {
	type subRow struct {
		ID          uint
		SubmitterID uint
		ReviewerID  *uint
	}
	var lastID uint
	for {
		var rows []subRow
		if err := db.Table("submissions").
			Select("id, submitter_id, reviewer_id").
			Where("id > ? AND deleted_at IS NULL", lastID).
			Order("id").
			Limit(syncBatchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		var rels []authz.Relationship
		for _, r := range rows {
			if r.SubmitterID > 0 {
				rels = append(rels, authz.Relationship{
					Subject:  authz.SubjectUser(r.SubmitterID),
					Relation: "submitter",
					Resource: authz.ResourceID("submission", r.ID),
				})
			}
			if r.ReviewerID != nil && *r.ReviewerID > 0 {
				rels = append(rels, authz.Relationship{
					Subject:  authz.SubjectUser(*r.ReviewerID),
					Relation: "reviewer",
					Resource: authz.ResourceID("submission", r.ID),
				})
			}
			lastID = r.ID
		}
		if err := grant(ctx, rels); err != nil {
			return err
		}
	}
}
