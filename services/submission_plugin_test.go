package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResourceInstanceCreator stands in for the plugin manager and records
// what the platform sent to the plugin on approval.
type fakeResourceInstanceCreator struct {
	calls    int
	pluginID uint
	slug     string
	payload  []byte
	reviewer uint
	err      error
}

func (f *fakeResourceInstanceCreator) CreateResourceInstance(pluginID uint, slug string, payload []byte, reviewerID uint) (*pb.ResourceInstanceProto, error) {
	f.calls++
	f.pluginID, f.slug, f.payload, f.reviewer = pluginID, slug, payload, reviewerID
	if f.err != nil {
		return nil, f.err
	}
	return &pb.ResourceInstanceProto{Id: "asset-123", Name: "Triage Agent", IsActive: true}, nil
}

const agentSchema = `{"type":"object","properties":{"name":{"type":"string","minLength":1},"purpose":{"type":"string"}},"required":["name","purpose"]}`

func setupPluginSubmissionTest(t *testing.T) (*Service, *models.PluginResourceType, *models.User, *models.User) {
	t.Helper()
	db := setupTestDBForSubmissions(t)
	service := NewService(db)
	service.NotificationService = NewTestNotificationService(db)

	plugin := &models.Plugin{Name: "asset-catalog", Command: "/usr/bin/asset-catalog", HookType: models.HookTypeResourceProvider, IsActive: true}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, service.RegisterPluginResourceTypes(plugin.ID, []models.PluginResourceType{
		{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: agentSchema},
		{Slug: "internal", Name: "Internal", SupportsSubmissions: false},
	}))
	prt, err := service.GetPluginResourceTypeByPluginAndSlug(plugin.ID, "agent")
	require.NoError(t, err)

	user := createSubmissionTestUser(t, service, "submitter@test.com")
	admin := createSubmissionTestAdmin(t, service, "admin@test.com")
	return service, prt, user, admin
}

func TestCreatePluginSubmission(t *testing.T) {
	service, prt, user, _ := setupPluginSubmissionTest(t)

	t.Run("creates a submitted plugin submission and notifies admins", func(t *testing.T) {
		service.NotificationService.ClearNotifications()
		sub, err := service.CreatePluginSubmission(user.ID, prt.ID, models.SubmissionStatusSubmitted,
			models.JSONMap{"name": "Triage Agent", "purpose": "Route tickets"},
			nil, 10, "Internal only", "contact@test.com", "", "", nil, "", "")
		require.NoError(t, err)
		assert.Equal(t, models.SubmissionResourceTypePlugin, sub.ResourceType)
		require.NotNil(t, sub.PluginResourceTypeID)
		assert.Equal(t, prt.ID, *sub.PluginResourceTypeID)
		assert.Equal(t, models.SubmissionStatusSubmitted, sub.Status)

		loaded, err := service.GetSubmissionByID(sub.ID)
		require.NoError(t, err)
		require.NotNil(t, loaded.PluginResourceType, "type is preloaded")
		assert.Equal(t, "Agent", loaded.PluginResourceType.Name)

		notes := service.NotificationService.GetNotifications()
		require.NotEmpty(t, notes, "admins are notified of a new submission")
		assert.Contains(t, notes[0].Title, "Agent")
		assert.Contains(t, notes[0].Content, "Triage Agent")
	})

	t.Run("rejects payloads that violate the schema", func(t *testing.T) {
		_, err := service.CreatePluginSubmission(user.ID, prt.ID, models.SubmissionStatusDraft,
			models.JSONMap{"name": "No purpose"},
			nil, 10, "", "contact@test.com", "", "", nil, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "purpose")
	})

	t.Run("rejects types that do not accept submissions", func(t *testing.T) {
		internal, err := service.GetPluginResourceTypeByPluginAndSlug(prt.PluginID, "internal")
		require.NoError(t, err)
		_, err = service.CreatePluginSubmission(user.ID, internal.ID, models.SubmissionStatusDraft,
			models.JSONMap{"name": "x"}, nil, 10, "", "", "", "", nil, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not accept")
	})

	t.Run("rejects unknown and inactive types", func(t *testing.T) {
		_, err := service.CreatePluginSubmission(user.ID, 9999, models.SubmissionStatusDraft,
			models.JSONMap{"name": "x"}, nil, 10, "", "", "", "", nil, "", "")
		require.Error(t, err)

		_, err = service.DeactivatePluginResourceTypesExcept(prt.PluginID, []string{"internal"})
		require.NoError(t, err)
		_, err = service.CreatePluginSubmission(user.ID, prt.ID, models.SubmissionStatusDraft,
			models.JSONMap{"name": "x", "purpose": "y"}, nil, 10, "", "", "", "", nil, "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not active")
		require.NoError(t, service.RegisterPluginResourceTypes(prt.PluginID, []models.PluginResourceType{
			{Slug: "agent", Name: "Agent", SupportsSubmissions: true, SubmissionSchema: agentSchema},
		}))
	})

	t.Run("CreateSubmission still refuses the plugin type", func(t *testing.T) {
		_, err := service.CreateSubmission(user.ID, models.SubmissionResourceTypePlugin, models.SubmissionStatusDraft,
			models.JSONMap{"name": "x"}, nil, 10, "", "", "", "", nil, "", "")
		require.Error(t, err)
	})

	t.Run("updates are re-validated against the schema", func(t *testing.T) {
		sub, err := service.CreatePluginSubmission(user.ID, prt.ID, models.SubmissionStatusDraft,
			models.JSONMap{"name": "Draft", "purpose": "tbd"}, nil, 10, "", "", "", "", nil, "", "")
		require.NoError(t, err)

		_, err = service.UpdateSubmission(sub.ID, user.ID, models.JSONMap{"name": ""}, nil, 10, "", "", "", "", nil, "", "")
		require.Error(t, err)

		updated, err := service.UpdateSubmission(sub.ID, user.ID, models.JSONMap{"name": "Draft v2", "purpose": "tbd"}, nil, 10, "", "", "", "", nil, "", "")
		require.NoError(t, err)
		assert.Equal(t, "Draft v2", updated.ResourcePayload["name"])
	})
}

func TestApprovePluginSubmission(t *testing.T) {
	service, prt, user, admin := setupPluginSubmissionTest(t)

	newSubmission := func(t *testing.T) *models.Submission {
		sub, err := service.CreatePluginSubmission(user.ID, prt.ID, models.SubmissionStatusSubmitted,
			models.JSONMap{"name": "Triage Agent", "purpose": "Route tickets"},
			models.JSONMap{"accepted": []interface{}{"terms"}}, 10, "Internal only",
			"contact@test.com", "", "", nil, "https://docs.example.com", "please review")
		require.NoError(t, err)
		return sub
	}

	t.Run("calls the plugin with the submission envelope and records the instance", func(t *testing.T) {
		fake := &fakeResourceInstanceCreator{}
		service.PluginResourceRPC = fake
		sub := newSubmission(t)

		approved, err := service.ApproveSubmission(sub.ID, admin.ID, 35, models.JSONMap{"ids": []interface{}{float64(7)}}, "fine")
		require.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusApproved, approved.Status)
		assert.Nil(t, approved.ResourceID, "plugin instances are not numeric resources")
		assert.Equal(t, "asset-123", approved.PluginInstanceID)

		assert.Equal(t, 1, fake.calls)
		assert.Equal(t, prt.PluginID, fake.pluginID)
		assert.Equal(t, "agent", fake.slug)
		assert.Equal(t, admin.ID, fake.reviewer)

		env, err := plugin_sdk.ParseSubmissionEnvelope(fake.payload)
		require.NoError(t, err)
		assert.Equal(t, "submission", env.Source)
		assert.Equal(t, uint32(sub.ID), env.SubmissionID)
		assert.Equal(t, uint32(user.ID), env.Submitter.ID)
		assert.Equal(t, user.Email, env.Submitter.Email)
		assert.Equal(t, uint32(admin.ID), env.Reviewer.ID)
		assert.Equal(t, admin.Email, env.Reviewer.Email)
		assert.Equal(t, 35, env.FinalPrivacyScore)
		assert.Equal(t, 10, env.SuggestedPrivacy)
		assert.Equal(t, "Internal only", env.PrivacyJustification)
		assert.Equal(t, []uint32{7}, env.AssignedCatalogues)
		assert.Equal(t, "https://docs.example.com", env.DocumentationURL)
		assert.Equal(t, "please review", env.Notes)
		assert.Equal(t, "Triage Agent", env.ResourcePayload["name"])
		assert.NotNil(t, env.Attestations)

		// Persisted
		reloaded, err := service.GetSubmissionByID(sub.ID)
		require.NoError(t, err)
		assert.Equal(t, "asset-123", reloaded.PluginInstanceID)
		assert.Equal(t, models.SubmissionStatusApproved, reloaded.Status)

		// Envelope is plain JSON a plugin can decode without the SDK too
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(fake.payload, &raw))
		assert.Equal(t, "submission", raw["source"])
	})

	t.Run("plugin rejection leaves the submission untouched", func(t *testing.T) {
		fake := &fakeResourceInstanceCreator{err: fmt.Errorf("duplicate name")}
		service.PluginResourceRPC = fake
		sub := newSubmission(t)

		_, err := service.ApproveSubmission(sub.ID, admin.ID, 35, nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate name")

		reloaded, err := service.GetSubmissionByID(sub.ID)
		require.NoError(t, err)
		assert.Equal(t, models.SubmissionStatusSubmitted, reloaded.Status)
		assert.Empty(t, reloaded.PluginInstanceID)
	})

	t.Run("fails cleanly when no plugin manager is available", func(t *testing.T) {
		service.PluginResourceRPC = nil
		service.AIStudioPluginManager = nil
		sub := newSubmission(t)

		_, err := service.ApproveSubmission(sub.ID, admin.ID, 35, nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plugin manager not available")
	})
}
