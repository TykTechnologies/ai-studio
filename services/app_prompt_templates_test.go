package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/guardrails/templates"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/eventbridge"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	ptIntro = "You are reviewing a pull request. Follow the review rules below.\n<diff>\n"
	ptOutro = "\n</diff>\nReport only real defects, one per line, with file and line.\n"
)

func setupPromptTemplateService(t *testing.T) (*Service, uint) {
	t.Helper()
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.App{}, &models.Credential{}, &models.AppPromptTemplate{}))
	// One connection: a second would open a new, empty in-memory database
	// (Matched writes from a goroutine of its own).
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	svc := NewService(db)
	app := &models.App{Name: "pr-review bot", UserID: 1}
	require.NoError(t, app.Create(db))
	return svc, app.ID
}

func prReviewSpec(name string) PromptTemplateSpec {
	return PromptTemplateSpec{
		Name:      name,
		Placement: &templates.Placement{Role: templates.RoleUser},
		Parts:     []templates.Part{{Const: ptIntro}, {Slot: "diff"}, {Const: ptOutro}},
	}
}

func TestAppPromptTemplates_CreateValidates(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)

	tpl, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("pr-review"))
	require.NoError(t, err)
	assert.Equal(t, 1, tpl.Version)
	assert.Equal(t, templates.StatusProposed, tpl.Status)
	assert.Equal(t, templates.Placement{Role: "user", Message: "any", Anchor: "both"}, tpl.Placement(), "placement defaults filled")
	assert.Equal(t, models.DefaultPromptTemplateExpiryDays, tpl.ExpiresAfterDays)

	bad := []struct {
		name string
		spec PromptTemplateSpec
	}{
		{"short const", PromptTemplateSpec{Name: "x", Placement: &templates.Placement{Role: "user"}, Parts: []templates.Part{{Const: "too short"}}}},
		{"bad name", func() PromptTemplateSpec { s := prReviewSpec("pr review!"); return s }()},
		{"no placement", PromptTemplateSpec{Name: "x", Parts: []templates.Part{{Const: ptIntro}}}},
		{"developer role", func() PromptTemplateSpec {
			s := prReviewSpec("x")
			s.Placement = &templates.Placement{Role: "developer"}
			return s
		}()},
		{"zero width in const", func() PromptTemplateSpec {
			s := prReviewSpec("x")
			s.Parts[0].Const = "You are reviewing\u200b a pull request; follow the rules."
			return s
		}()},
		{"expiry out of range", func() PromptTemplateSpec { s := prReviewSpec("x"); s.ExpiresAfterDays = new(0); return s }()},
	}
	for _, tc := range bad {
		_, err := svc.CreateAppPromptTemplate(appID, 1, tc.spec)
		assert.ErrorIs(t, err, ErrInvalidPromptTemplate, tc.name)
	}

	_, err = svc.CreateAppPromptTemplate(99999, 1, prReviewSpec("pr-review"))
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "unknown app")

	again, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("pr-review"))
	require.NoError(t, err)
	assert.Equal(t, 2, again.Version, "a second proposal under a name is its next version")
}

func TestAppPromptTemplates_ApproveAndVersioning(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	v1, err := svc.CreateAppPromptTemplate(appID, 7, prReviewSpec("pr-review"))
	require.NoError(t, err)

	assert.Empty(t, svc.PromptTemplateSource().ApprovedTemplates(context.Background(), appID), "a proposed template is never matched")

	approved, err := svc.ApproveAppPromptTemplate(appID, v1.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, templates.StatusApproved, approved.Status)
	require.NotNil(t, approved.ApprovedBy)
	assert.Equal(t, uint(1), *approved.ApprovedBy)
	assert.NotNil(t, approved.ApprovedAt)

	_, err = svc.ApproveAppPromptTemplate(appID, v1.ID, 1)
	assert.ErrorIs(t, err, ErrPromptTemplateConflict, "approving twice")

	got := svc.PromptTemplateSource().ApprovedTemplates(context.Background(), appID)
	require.Len(t, got, 1)
	assert.Equal(t, "pr-review@1", got[0].Ref())

	// Expiry changes in place: it does not change what is trusted.
	upd, err := svc.UpdateAppPromptTemplate(appID, v1.ID, 7, PromptTemplateSpec{ExpiresAfterDays: new(60)})
	require.NoError(t, err)
	assert.Equal(t, v1.ID, upd.ID)
	assert.Equal(t, templates.StatusApproved, upd.Status)

	// Changing a constant of an approved template makes the next version,
	// proposed; the approved one stays as it was.
	changed := prReviewSpec("")
	changed.Parts[2].Const = ptOutro + "Never approve the change yourself.\n"
	v2, err := svc.UpdateAppPromptTemplate(appID, v1.ID, 7, PromptTemplateSpec{Parts: changed.Parts})
	require.NoError(t, err)
	assert.NotEqual(t, v1.ID, v2.ID)
	assert.Equal(t, 2, v2.Version)
	assert.Equal(t, templates.StatusProposed, v2.Status)
	assert.Nil(t, v2.ApprovedBy)
	still, err := svc.GetAppPromptTemplate(appID, v1.ID)
	require.NoError(t, err)
	assert.Equal(t, templates.StatusApproved, still.Status)
	assert.Equal(t, ptOutro, still.Parts[2].Const)

	// So does a placement change.
	v3, err := svc.UpdateAppPromptTemplate(appID, v1.ID, 7, PromptTemplateSpec{Placement: &templates.Placement{Role: "system"}})
	require.NoError(t, err)
	assert.Equal(t, 3, v3.Version)

	// A proposed version changes in place.
	v2b, err := svc.UpdateAppPromptTemplate(appID, v2.ID, 7, PromptTemplateSpec{Parts: prReviewSpec("").Parts})
	require.NoError(t, err)
	assert.Equal(t, v2.ID, v2b.ID)

	// Approving v2 retires v1: one version of a name is trusted at a time.
	_, err = svc.ApproveAppPromptTemplate(appID, v2.ID, 1)
	require.NoError(t, err)
	old, err := svc.GetAppPromptTemplate(appID, v1.ID)
	require.NoError(t, err)
	assert.Equal(t, templates.StatusRetired, old.Status)
	got = svc.PromptTemplateSource().ApprovedTemplates(context.Background(), appID)
	require.Len(t, got, 1)
	assert.Equal(t, "pr-review@2", got[0].Ref())

	_, err = svc.UpdateAppPromptTemplate(appID, v2.ID, 7, PromptTemplateSpec{Name: "renamed"})
	assert.ErrorIs(t, err, ErrInvalidPromptTemplate)

	retired, err := svc.RetireAppPromptTemplate(appID, v2.ID)
	require.NoError(t, err)
	assert.Equal(t, templates.StatusRetired, retired.Status)
	assert.Empty(t, svc.PromptTemplateSource().ApprovedTemplates(context.Background(), appID))

	_, err = svc.ApproveAppPromptTemplate(appID, v2.ID, 1)
	assert.ErrorIs(t, err, ErrPromptTemplateConflict, "a retired version cannot be approved again")

	require.NoError(t, svc.DeleteAppPromptTemplate(appID, v3.ID))
	_, err = svc.GetAppPromptTemplate(appID, v3.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = svc.GetAppPromptTemplate(appID+1, v1.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "templates are scoped to their app")
}

func TestAppPromptTemplates_ApprovedCap(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	for i := range templates.MaxApprovedPerApp {
		tpl, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec(fmt.Sprintf("t%d", i)))
		require.NoError(t, err)
		_, err = svc.ApproveAppPromptTemplate(appID, tpl.ID, 1)
		require.NoError(t, err)
	}
	extra, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("one-too-many"))
	require.NoError(t, err)
	_, err = svc.ApproveAppPromptTemplate(appID, extra.ID, 1)
	require.ErrorIs(t, err, ErrPromptTemplateConflict)
	assert.Contains(t, err.Error(), "at most 10")

	// A new version of an approved name replaces it, so it fits.
	next, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("t0"))
	require.NoError(t, err)
	_, err = svc.ApproveAppPromptTemplate(appID, next.ID, 1)
	require.NoError(t, err)
	n, err := models.CountApprovedAppPromptTemplates(svc.DB, appID)
	require.NoError(t, err)
	assert.EqualValues(t, templates.MaxApprovedPerApp, n)
}

func TestAppPromptTemplates_Lapse(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	tpl, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("pr-review"))
	require.NoError(t, err)
	_, err = svc.ApproveAppPromptTemplate(appID, tpl.ID, 1)
	require.NoError(t, err)

	// Approved 31 days ago, never matched: it lapses back to proposed.
	old := time.Now().Add(-31 * 24 * time.Hour)
	require.NoError(t, svc.DB.Model(&models.AppPromptTemplate{}).Where("id = ?", tpl.ID).Update("approved_at", old).Error)
	got, err := svc.ApprovedAppPromptTemplates(appID)
	require.NoError(t, err)
	assert.Empty(t, got)
	lapsed, err := svc.GetAppPromptTemplate(appID, tpl.ID)
	require.NoError(t, err)
	assert.Equal(t, templates.StatusProposed, lapsed.Status)
	assert.Nil(t, lapsed.ApprovedAt)

	// A recent match keeps an old approval alive.
	_, err = svc.ApproveAppPromptTemplate(appID, tpl.ID, 1)
	require.NoError(t, err)
	recent := time.Now().Add(-2 * time.Hour)
	require.NoError(t, svc.DB.Model(&models.AppPromptTemplate{}).Where("id = ?", tpl.ID).Updates(map[string]any{"approved_at": old, "last_matched_at": recent}).Error)
	got, err = svc.ApprovedAppPromptTemplates(appID)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestAppPromptTemplates_SourceCachesAndTouches(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	src := svc.PromptTemplateSource()
	assert.Nil(t, src.ApprovedTemplates(context.Background(), 0), "no app, no templates")
	assert.Nil(t, src.ApprovedTemplates(context.Background(), appID))

	tpl, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("pr-review"))
	require.NoError(t, err)
	_, err = svc.ApproveAppPromptTemplate(appID, tpl.ID, 1)
	require.NoError(t, err)
	got := src.ApprovedTemplates(context.Background(), appID)
	require.Len(t, got, 1, "a write on this replica drops the cached entry")

	src.Matched(got[0])
	require.Eventually(t, func() bool {
		row, err := svc.GetAppPromptTemplate(appID, tpl.ID)
		return err == nil && row.LastMatchedAt != nil
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAppPromptTemplates_ErrorsAreDistinct(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	_, err := svc.CreateAppPromptTemplate(appID, 1, PromptTemplateSpec{Name: strings.Repeat("a", 200), Placement: &templates.Placement{Role: "user"}})
	assert.True(t, errors.Is(err, ErrInvalidPromptTemplate))
	assert.False(t, errors.Is(err, ErrPromptTemplateConflict))
}

// Every template change announces itself: the approved templates travel in
// the edge snapshot, and the control server recomputes the edges' checksum
// on this topic.
func TestAppPromptTemplates_EmitChangeEvents(t *testing.T) {
	svc, appID := setupPromptTemplateService(t)
	bus := eventbridge.NewBus()
	svc.SystemEvents = NewSystemEventEmitter(bus, "test")
	assert.Equal(t, "system.app.prompt_templates.changed", TopicAppPromptTemplatesChanged)

	collector := NewTestEventCollector()
	collector.Expect(4)
	sub := bus.Subscribe(TopicAppPromptTemplatesChanged, collector.Handler())
	defer bus.Unsubscribe(sub)

	tpl, err := svc.CreateAppPromptTemplate(appID, 1, prReviewSpec("pr-review"))
	require.NoError(t, err)
	_, err = svc.ApproveAppPromptTemplate(appID, tpl.ID, 1)
	require.NoError(t, err)
	_, err = svc.RetireAppPromptTemplate(appID, tpl.ID)
	require.NoError(t, err)
	require.NoError(t, svc.DeleteAppPromptTemplate(appID, tpl.ID))

	require.True(t, collector.WaitWithTimeout(2*time.Second))
	for _, ev := range collector.GetEvents() {
		payload := parseEventPayload(t, ev)
		assert.Equal(t, "app", payload.ObjectType)
		assert.Equal(t, appID, payload.ObjectID)
	}
}
