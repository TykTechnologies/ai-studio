package openfga

import (
	"context"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/authz"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FullSync reads the entire GORM database and populates the authorization store with all
// relationships. This is called at startup and can be called periodically as a
// consistency safety net.
func (s *Store) FullSync(ctx context.Context, db *gorm.DB) error {
	log.Info().Msg("authz: starting full relationship sync from database")

	var rels []authz.Relationship

	var err error
	if rels, err = collectSystemRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync system relationships: %w", err)
	}
	if rels, err = collectGroupMemberRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync group members: %w", err)
	}
	if rels, err = collectCatalogueRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync catalogues: %w", err)
	}
	if rels, err = collectDataCatalogueRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync data catalogues: %w", err)
	}
	if rels, err = collectToolCatalogueRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync tool catalogues: %w", err)
	}
	if rels, err = collectLLMRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync llms: %w", err)
	}
	if rels, err = collectDatasourceRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync datasources: %w", err)
	}
	if rels, err = collectToolRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync tools: %w", err)
	}
	if rels, err = collectAppRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync apps: %w", err)
	}
	if rels, err = collectChatRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync chats: %w", err)
	}
	if rels, err = collectPluginResourceRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync plugin resources: %w", err)
	}
	if rels, err = collectSubmissionRels(db, rels); err != nil {
		return fmt.Errorf("authz: sync submissions: %w", err)
	}

	if len(rels) == 0 {
		log.Info().Msg("authz: no relationships to sync (empty database)")
		return nil
	}

	if err := s.Grant(ctx, rels); err != nil {
		return fmt.Errorf("authz: failed to write relationships during sync: %w", err)
	}

	log.Info().Int("count", len(rels)).Msg("authz: full sync complete")
	return nil
}

// --- Relationship collectors ---
// Each collector queries a set of GORM tables and appends relationships to the slice.

// joinRow is a generic struct for scanning join table rows.
type joinRow struct {
	LeftID  uint
	RightID uint
}

func collectSystemRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	// Users with IsAdmin=true -> system:1#admin
	var adminIDs []uint
	if err := db.Table("users").
		Where("is_admin = ? AND deleted_at IS NULL", true).
		Pluck("id", &adminIDs).Error; err != nil {
		return rels, err
	}
	for _, id := range adminIDs {
		rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(id), Relation: "admin", Resource: "system:1"})
	}

	// Users with AccessToSSOConfig=true -> system:1#sso_admin
	var ssoIDs []uint
	if err := db.Table("users").
		Where("access_to_sso_config = ? AND deleted_at IS NULL", true).
		Pluck("id", &ssoIDs).Error; err != nil {
		return rels, err
	}
	for _, id := range ssoIDs {
		rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(id), Relation: "sso_admin", Resource: "system:1"})
	}

	// All active users are system members.
	var allIDs []uint
	if err := db.Table("users").
		Where("deleted_at IS NULL").
		Pluck("id", &allIDs).Error; err != nil {
		return rels, err
	}
	for _, id := range allIDs {
		rels = append(rels, authz.Relationship{Subject: authz.SubjectUser(id), Relation: "member", Resource: "system:1"})
	}

	return rels, nil
}

func collectGroupMemberRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("user_groups").Select("group_id as left_id, user_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectUser(r.RightID),
			Relation: "member",
			Resource: authz.SubjectGroup(r.LeftID),
		})
	}
	return rels, nil
}

func collectCatalogueRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("group_catalogues").Select("group_id as left_id, catalogue_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectGroup(r.LeftID),
			Relation: "assigned_group",
			Resource: authz.ResourceID("catalogue", r.RightID),
		})
	}
	return rels, nil
}

func collectDataCatalogueRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("group_datacatalogues").Select("group_id as left_id, data_catalogue_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectGroup(r.LeftID),
			Relation: "assigned_group",
			Resource: authz.ResourceID("data_catalogue", r.RightID),
		})
	}
	return rels, nil
}

func collectToolCatalogueRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("group_toolcatalogues").Select("group_id as left_id, tool_catalogue_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectGroup(r.LeftID),
			Relation: "assigned_group",
			Resource: authz.ResourceID("tool_catalogue", r.RightID),
		})
	}
	return rels, nil
}

func collectLLMRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("catalogue_llms").Select("catalogue_id as left_id, llm_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.ResourceID("catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Resource: authz.ResourceID("llm", r.RightID),
		})
	}
	return rels, nil
}

func collectDatasourceRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("data_catalogue_data_sources").Select("data_catalogue_id as left_id, datasource_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.ResourceID("data_catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Resource: authz.ResourceID("datasource", r.RightID),
		})
	}

	type ownerRow struct {
		ID     uint
		UserID uint
	}
	var owners []ownerRow
	if err := db.Table("datasources").Select("id, user_id").Where("user_id > 0 AND deleted_at IS NULL").Find(&owners).Error; err != nil {
		return rels, err
	}
	for _, o := range owners {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectUser(o.UserID),
			Relation: "owner",
			Resource: authz.ResourceID("datasource", o.ID),
		})
	}

	return rels, nil
}

func collectToolRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("tool_catalogue_tools").Select("tool_catalogue_id as left_id, tool_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.ResourceID("tool_catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Resource: authz.ResourceID("tool", r.RightID),
		})
	}
	return rels, nil
}

func collectAppRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	type ownerRow struct {
		ID     uint
		UserID uint
	}
	var owners []ownerRow
	if err := db.Table("apps").Select("id, user_id").Where("user_id > 0 AND deleted_at IS NULL").Find(&owners).Error; err != nil {
		return rels, err
	}
	for _, o := range owners {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectUser(o.UserID),
			Relation: "owner",
			Resource: authz.ResourceID("app", o.ID),
		})
	}
	return rels, nil
}

func collectChatRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	var rows []joinRow
	if err := db.Table("chat_groups").Select("group_id as left_id, chat_id as right_id").Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectGroup(r.LeftID),
			Relation: "assigned_group",
			Resource: authz.ResourceID("chat", r.RightID),
		})
	}
	return rels, nil
}

func collectPluginResourceRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	type pluginResRow struct {
		GroupID              uint
		PluginResourceTypeID uint
		InstanceID           string
	}
	var rows []pluginResRow
	if err := db.Table("group_plugin_resources").
		Select("group_id, plugin_resource_type_id, instance_id").
		Where("deleted_at IS NULL").
		Find(&rows).Error; err != nil {
		return rels, err
	}
	for _, r := range rows {
		if err := authz.ValidateID(r.InstanceID); err != nil {
			log.Warn().Err(err).
				Uint("group_id", r.GroupID).
				Uint("plugin_resource_type_id", r.PluginResourceTypeID).
				Str("instance_id", r.InstanceID).
				Msg("authz: skipping plugin resource with invalid instance ID")
			continue
		}
		objectID := fmt.Sprintf("%d_%s", r.PluginResourceTypeID, r.InstanceID)
		rels = append(rels, authz.Relationship{
			Subject:  authz.SubjectGroup(r.GroupID),
			Relation: "assigned_group",
			Resource: "plugin_resource:" + objectID,
		})
	}
	return rels, nil
}

func collectSubmissionRels(db *gorm.DB, rels []authz.Relationship) ([]authz.Relationship, error) {
	type subRow struct {
		ID          uint
		SubmitterID uint
		ReviewerID  *uint
	}
	var rows []subRow
	if err := db.Table("submissions").
		Select("id, submitter_id, reviewer_id").
		Where("deleted_at IS NULL").
		Find(&rows).Error; err != nil {
		return rels, err
	}
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
	}
	return rels, nil
}
