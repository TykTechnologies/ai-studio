package authz

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FullSync reads the entire GORM database and populates the OpenFGA store with all
// relationship tuples. This is called at startup and can be called periodically as a
// consistency safety net.
func (s *Store) FullSync(ctx context.Context, db *gorm.DB) error {
	log.Info().Msg("authz: starting full tuple sync from database")

	var tuples []Tuple

	var err error
	if tuples, err = collectSystemTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync system tuples: %w", err)
	}
	if tuples, err = collectGroupMemberTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync group members: %w", err)
	}
	if tuples, err = collectCatalogueTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync catalogues: %w", err)
	}
	if tuples, err = collectDataCatalogueTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync data catalogues: %w", err)
	}
	if tuples, err = collectToolCatalogueTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync tool catalogues: %w", err)
	}
	if tuples, err = collectLLMTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync llms: %w", err)
	}
	if tuples, err = collectDatasourceTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync datasources: %w", err)
	}
	if tuples, err = collectToolTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync tools: %w", err)
	}
	if tuples, err = collectAppTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync apps: %w", err)
	}
	if tuples, err = collectChatTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync chats: %w", err)
	}
	if tuples, err = collectPluginResourceTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync plugin resources: %w", err)
	}
	if tuples, err = collectSubmissionTuples(db, tuples); err != nil {
		return fmt.Errorf("authz: sync submissions: %w", err)
	}

	if len(tuples) == 0 {
		log.Info().Msg("authz: no tuples to sync (empty database)")
		return nil
	}

	if err := s.WriteTuples(ctx, tuples); err != nil {
		return fmt.Errorf("authz: failed to write tuples during sync: %w", err)
	}

	log.Info().Int("tuple_count", len(tuples)).Msg("authz: full sync complete")
	return nil
}

// --- Tuple collectors ---
// Each collector queries a set of GORM tables and appends tuples to the slice.

// joinRow is a generic struct for scanning join table rows.
type joinRow struct {
	LeftID  uint
	RightID uint
}

func collectSystemTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// Users with IsAdmin=true -> system:1#admin
	var adminIDs []uint
	if err := db.Table("users").
		Where("is_admin = ? AND deleted_at IS NULL", true).
		Pluck("id", &adminIDs).Error; err != nil {
		return tuples, err
	}
	for _, id := range adminIDs {
		tuples = append(tuples, Tuple{User: UserStr(id), Relation: "admin", Object: "system:1"})
	}

	// Users with AccessToSSOConfig=true -> system:1#sso_admin
	var ssoIDs []uint
	if err := db.Table("users").
		Where("access_to_sso_config = ? AND deleted_at IS NULL", true).
		Pluck("id", &ssoIDs).Error; err != nil {
		return tuples, err
	}
	for _, id := range ssoIDs {
		tuples = append(tuples, Tuple{User: UserStr(id), Relation: "sso_admin", Object: "system:1"})
	}

	// All active users are system members.
	var allIDs []uint
	if err := db.Table("users").
		Where("deleted_at IS NULL").
		Pluck("id", &allIDs).Error; err != nil {
		return tuples, err
	}
	for _, id := range allIDs {
		tuples = append(tuples, Tuple{User: UserStr(id), Relation: "member", Object: "system:1"})
	}

	return tuples, nil
}

func collectGroupMemberTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	var rows []joinRow
	if err := db.Table("user_groups").Select("group_id as left_id, user_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     UserStr(r.RightID),
			Relation: "member",
			Object:   GroupStr(r.LeftID),
		})
	}
	return tuples, nil
}

func collectCatalogueTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// group_catalogues -> catalogue#assigned_group
	var rows []joinRow
	if err := db.Table("group_catalogues").Select("group_id as left_id, catalogue_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     GroupStr(r.LeftID),
			Relation: "assigned_group",
			Object:   ObjectStr("catalogue", r.RightID),
		})
	}
	return tuples, nil
}

func collectDataCatalogueTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	var rows []joinRow
	if err := db.Table("group_datacatalogues").Select("group_id as left_id, data_catalogue_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     GroupStr(r.LeftID),
			Relation: "assigned_group",
			Object:   ObjectStr("data_catalogue", r.RightID),
		})
	}
	return tuples, nil
}

func collectToolCatalogueTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	var rows []joinRow
	if err := db.Table("group_toolcatalogues").Select("group_id as left_id, tool_catalogue_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     GroupStr(r.LeftID),
			Relation: "assigned_group",
			Object:   ObjectStr("tool_catalogue", r.RightID),
		})
	}
	return tuples, nil
}

func collectLLMTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// catalogue_llms -> llm#parent_catalogue
	var rows []joinRow
	if err := db.Table("catalogue_llms").Select("catalogue_id as left_id, llm_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     ObjectStr("catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Object:   ObjectStr("llm", r.RightID),
		})
	}
	return tuples, nil
}

func collectDatasourceTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// data_catalogue_data_sources -> datasource#parent_catalogue
	var rows []joinRow
	if err := db.Table("data_catalogue_data_sources").Select("data_catalogue_id as left_id, datasource_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     ObjectStr("data_catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Object:   ObjectStr("datasource", r.RightID),
		})
	}

	// datasource.user_id -> datasource#owner
	type ownerRow struct {
		ID     uint
		UserID uint
	}
	var owners []ownerRow
	if err := db.Table("datasources").Select("id, user_id").Where("user_id > 0 AND deleted_at IS NULL").Find(&owners).Error; err != nil {
		return tuples, err
	}
	for _, o := range owners {
		tuples = append(tuples, Tuple{
			User:     UserStr(o.UserID),
			Relation: "owner",
			Object:   ObjectStr("datasource", o.ID),
		})
	}

	return tuples, nil
}

func collectToolTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// tool_catalogue_tools -> tool#parent_catalogue
	var rows []joinRow
	if err := db.Table("tool_catalogue_tools").Select("tool_catalogue_id as left_id, tool_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     ObjectStr("tool_catalogue", r.LeftID),
			Relation: "parent_catalogue",
			Object:   ObjectStr("tool", r.RightID),
		})
	}
	return tuples, nil
}

func collectAppTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// app.user_id -> app#owner
	type ownerRow struct {
		ID     uint
		UserID uint
	}
	var owners []ownerRow
	if err := db.Table("apps").Select("id, user_id").Where("user_id > 0 AND deleted_at IS NULL").Find(&owners).Error; err != nil {
		return tuples, err
	}
	for _, o := range owners {
		tuples = append(tuples, Tuple{
			User:     UserStr(o.UserID),
			Relation: "owner",
			Object:   ObjectStr("app", o.ID),
		})
	}
	return tuples, nil
}

func collectChatTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
	// chat_groups -> chat#assigned_group
	var rows []joinRow
	if err := db.Table("chat_groups").Select("group_id as left_id, chat_id as right_id").Find(&rows).Error; err != nil {
		return tuples, err
	}
	for _, r := range rows {
		tuples = append(tuples, Tuple{
			User:     GroupStr(r.LeftID),
			Relation: "assigned_group",
			Object:   ObjectStr("chat", r.RightID),
		})
	}
	return tuples, nil
}

func collectPluginResourceTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
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
		return tuples, err
	}
	for _, r := range rows {
		objectID := fmt.Sprintf("%d_%s", r.PluginResourceTypeID, r.InstanceID)
		tuples = append(tuples, Tuple{
			User:     GroupStr(r.GroupID),
			Relation: "assigned_group",
			Object:   "plugin_resource:" + objectID,
		})
	}
	return tuples, nil
}

func collectSubmissionTuples(db *gorm.DB, tuples []Tuple) ([]Tuple, error) {
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
		return tuples, err
	}
	for _, r := range rows {
		if r.SubmitterID > 0 {
			tuples = append(tuples, Tuple{
				User:     UserStr(r.SubmitterID),
				Relation: "submitter",
				Object:   ObjectStr("submission", r.ID),
			})
		}
		if r.ReviewerID != nil && *r.ReviewerID > 0 {
			tuples = append(tuples, Tuple{
				User:     UserStr(*r.ReviewerID),
				Relation: "reviewer",
				Object:   ObjectStr("submission", r.ID),
			})
		}
	}
	return tuples, nil
}
