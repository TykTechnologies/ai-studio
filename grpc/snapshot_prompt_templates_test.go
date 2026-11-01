package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

const (
	snapTplIntro = "You are reviewing a pull request. Follow the review rules below.\n<diff>\n"
	snapTplOutro = "\n</diff>\nReport only real defects, one per line, with file and line.\n"
)

func createPromptTemplate(t *testing.T, db *gorm.DB, appID uint, name, status string) *models.AppPromptTemplate {
	t.Helper()
	now := time.Now()
	tpl := &models.AppPromptTemplate{
		AppID: appID, Name: name, Version: 1, Status: status, ExpiresAfterDays: 30,
		Parts: []templates.Part{{Const: snapTplIntro}, {Slot: "diff"}, {Const: snapTplOutro}},
	}
	tpl.SetPlacement(templates.Placement{Role: templates.RoleUser}.Normalize())
	if status == templates.StatusApproved {
		tpl.ApprovedAt = &now
	}
	require.NoError(t, tpl.Create(db))
	return tpl
}

func snapshotTemplateIDs(snap *pb.ConfigurationSnapshot) []uint32 {
	var out []uint32
	for _, t := range snap.PromptTemplates {
		out = append(out, t.Id)
	}
	return out
}

// The snapshot carries the approved, unlapsed templates of the Apps it
// carries, and only those: proposed, retired and lapsed templates, those of
// inactive Apps and those of Apps in another namespace stay on the hub.
func TestSnapshot_PromptTemplatesIncludedAndExcluded(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	other := &models.App{Name: "Other namespace", IsActive: true, Namespace: "eu"}
	require.NoError(t, db.Create(other).Error)
	inactive := &models.App{Name: "Inactive", IsActive: true}
	require.NoError(t, db.Create(inactive).Error)
	require.NoError(t, db.Model(inactive).Update("is_active", false).Error)

	approved := createPromptTemplate(t, db, apps[0].ID, "pr-review", templates.StatusApproved)
	createPromptTemplate(t, db, apps[0].ID, "proposed", templates.StatusProposed)
	createPromptTemplate(t, db, apps[1].ID, "retired", templates.StatusRetired)
	lapsed := createPromptTemplate(t, db, apps[1].ID, "lapsed", templates.StatusApproved)
	long := time.Now().Add(-40 * 24 * time.Hour)
	require.NoError(t, db.Model(lapsed).Update("approved_at", long).Error)
	elsewhere := createPromptTemplate(t, db, other.ID, "eu-only", templates.StatusApproved)
	createPromptTemplate(t, db, inactive.ID, "inactive", templates.StatusApproved)

	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	require.Equal(t, []uint32{uint32(approved.ID)}, snapshotTemplateIDs(snap))
	got := snap.PromptTemplates[0]
	assert.Equal(t, uint32(apps[0].ID), got.AppId)
	assert.Equal(t, "pr-review", got.Name)
	assert.EqualValues(t, 1, got.Version)
	assert.Equal(t, []string{"user", "any", "both"}, []string{got.PlacementRole, got.PlacementMessage, got.PlacementAnchor})
	require.Len(t, got.Parts, 3)
	assert.Equal(t, snapTplIntro, got.Parts[0].Constant)
	assert.Equal(t, "diff", got.Parts[1].Slot)

	// The namespaced edge gets its own App's template and the global ones.
	snap, err = server.getConfigurationSnapshot("eu")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint32{uint32(approved.ID), uint32(elsewhere.ID)}, snapshotTemplateIDs(snap))
}

// Approving, retiring and deleting a template each change the checksum, so
// edges become pending; with no templates the snapshot has none and its
// checksum is what it was before templates existed.
func TestSnapshot_PromptTemplatesChangeTheChecksum(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)

	checksum := func() string {
		snap, err := server.getConfigurationSnapshot("default")
		require.NoError(t, err)
		return snap.Checksum
	}
	none := checksum()
	snap, err := server.getConfigurationSnapshot("default")
	require.NoError(t, err)
	assert.Nil(t, snap.PromptTemplates)

	tpl := createPromptTemplate(t, db, apps[0].ID, "pr-review", templates.StatusProposed)
	assert.Equal(t, none, checksum(), "a proposed template is not in the snapshot")

	now := time.Now()
	require.NoError(t, db.Model(tpl).Updates(map[string]any{"status": templates.StatusApproved, "approved_at": now}).Error)
	approved := checksum()
	assert.NotEqual(t, none, approved, "approving marks edges pending")

	require.NoError(t, db.Model(tpl).Update("last_matched_at", time.Now()).Error)
	assert.Equal(t, approved, checksum(), "a match does not change the snapshot")

	require.NoError(t, db.Model(tpl).Update("status", templates.StatusRetired).Error)
	assert.Equal(t, none, checksum(), "retiring marks edges pending")

	require.NoError(t, db.Model(tpl).Updates(map[string]any{"status": templates.StatusApproved}).Error)
	assert.Equal(t, approved, checksum())
	require.NoError(t, db.Delete(tpl).Error)
	assert.Equal(t, none, checksum(), "deleting marks edges pending")
}

// Edges report the templates that matched in their heartbeats; the hub
// records the match on approved templates only, at most once per interval.
func TestEdgeTemplateMatchesRecorded(t *testing.T) {
	server, db := setupTestServer(t, nil)
	llms := createTestLLMs(db, "")
	apps := createTestApps(db, "", llms)
	approved := createPromptTemplate(t, db, apps[0].ID, "pr-review", templates.StatusApproved)
	retired := createPromptTemplate(t, db, apps[0].ID, "old", templates.StatusRetired)
	edgeTemplateTouched.Delete(approved.ID)
	edgeTemplateTouched.Delete(retired.ID)
	t.Cleanup(func() {
		edgeTemplateTouched.Delete(approved.ID)
		edgeTemplateTouched.Delete(retired.ID)
	})

	server.recordEdgeTemplateMatches("edge-1", []uint32{uint32(approved.ID), uint32(retired.ID), 0})
	var got models.AppPromptTemplate
	require.NoError(t, db.First(&got, approved.ID).Error)
	require.NotNil(t, got.LastMatchedAt, "an edge match keeps the template from lapsing")
	first := *got.LastMatchedAt
	var old models.AppPromptTemplate
	require.NoError(t, db.First(&old, retired.ID).Error)
	assert.Nil(t, old.LastMatchedAt, "a template no longer approved is left alone")

	server.recordEdgeTemplateMatches("edge-2", []uint32{uint32(approved.ID)})
	got = models.AppPromptTemplate{}
	require.NoError(t, db.First(&got, approved.ID).Error)
	assert.True(t, got.LastMatchedAt.Equal(first), "throttled")
}
