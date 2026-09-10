//go:build enterprise
// +build enterprise

package grpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The plugin-facing surface a resource provider (e.g. an asset catalogue) relies
// on: self-referencing object types, audience-filtered reads with display
// values, schema payloads that include vocabulary terms, and deletion.
func TestGovernedMetadataServer_ResourceProviderSurface(t *testing.T) {
	srv, svc := newGovernedMetadataTestServer(t)
	db := svc.DB
	gm := svc.GovernedMetadata()

	plugin := &models.Plugin{Name: "asset-catalog"}
	require.NoError(t, db.Create(plugin).Error)
	require.NoError(t, db.Create(&models.PluginResourceType{PluginID: plugin.ID, Slug: "prompts", Name: "Prompts", SupportsMetadata: true, IsActive: true}).Error)
	require.NoError(t, db.Create(&models.User{Email: "owner@example.com", Name: "Prompt Owner"}).Error)
	pluginCtx := context.WithValue(context.Background(), pluginInfoKeyString, plugin)
	anon := context.Background()
	self := "plugin_resource:self:prompts"
	concrete := models.PluginResourceObjectType(plugin.ID, "prompts")

	require.NoError(t, gm.CreateVocabulary(&models.MetadataVocabulary{Name: "Risk", Slug: "risk_tier",
		Terms: []models.VocabularyTerm{{Value: "low", Label: "Low"}, {Value: "high", Label: "High"}}}))
	require.NoError(t, gm.CreateSchema(&models.MetadataSchema{Name: "Prompt governance", Slug: "pg", AppliesTo: []string{concrete}, Active: true,
		Fields: []models.MetadataFieldDef{
			{Key: "risk_tier", Type: "vocabulary", VocabularySlug: "risk_tier", Required: true, PortalVisible: true},
			{Key: "business_owner", Type: "user", PortalVisible: true},
			{Key: "internal_note", Type: "string", GatewayVisible: true},
		}}))

	t.Run("self requires a plugin context", func(t *testing.T) {
		_, err := srv.GetResolvedMetadataSchema(anon, &pb.GetResolvedMetadataSchemaRequest{ObjectType: self})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = srv.SetObjectMetadata(anon, &pb.SetObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1", ValuesJson: `{}`})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		_, err = srv.DeleteObjectMetadata(anon, &pb.DeleteObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("resolved schema ships fields, json schema and vocabulary terms", func(t *testing.T) {
		resolved, err := srv.GetResolvedMetadataSchema(pluginCtx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: self})
		require.NoError(t, err)
		assert.Equal(t, "advisory", resolved.Enforcement)
		assert.Equal(t, []string{"pg"}, resolved.SchemaSlugs)
		var fields []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(resolved.FieldsJson), &fields))
		require.Len(t, fields, 3)
		assert.Equal(t, "risk_tier", fields[0]["key"])
		var vocab map[string][]map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(resolved.VocabulariesJson), &vocab))
		require.Len(t, vocab["risk_tier"], 2)
		assert.Equal(t, "High", vocab["risk_tier"][1]["label"])
		assert.Contains(t, resolved.JsonSchema, `"enum"`)

		// An object type without fields still returns a well-formed payload.
		empty, err := srv.GetResolvedMetadataSchema(pluginCtx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "plugin_resource:self:drafts"})
		require.NoError(t, err)
		assert.Equal(t, "[]", empty.FieldsJson)
		assert.Equal(t, "{}", empty.VocabulariesJson)
	})

	t.Run("writes through self land on the concrete object type", func(t *testing.T) {
		set, err := srv.SetObjectMetadata(pluginCtx, &pb.SetObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1",
			ValuesJson: `{"risk_tier":"high","business_owner":1,"internal_note":"do not ship"}`})
		require.NoError(t, err)
		require.True(t, set.Success, set.Message)
		rec, err := gm.GetObjectMetadata(concrete, "ast_1")
		require.NoError(t, err)
		assert.Equal(t, "high", rec.Values["risk_tier"])
		assert.Equal(t, models.MetadataSourcePlugin(plugin.ID), rec.UpdatedBySource)
	})

	t.Run("audience-filtered reads", func(t *testing.T) {
		admin, err := srv.GetObjectMetadata(pluginCtx, &pb.GetObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1"})
		require.NoError(t, err)
		assert.JSONEq(t, `{"risk_tier":"high","business_owner":1,"internal_note":"do not ship"}`, admin.ValuesJson)
		assert.Empty(t, admin.DisplayJson, "display values are a portal concern")

		portal, err := srv.GetObjectMetadata(pluginCtx, &pb.GetObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1", Visibility: "portal"})
		require.NoError(t, err)
		assert.JSONEq(t, `{"risk_tier":"high","business_owner":1}`, portal.ValuesJson)
		var display []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(portal.DisplayJson), &display))
		require.Len(t, display, 2)
		assert.Equal(t, "High", display[0]["value"], "vocabulary label resolved")
		assert.Equal(t, "Prompt Owner", display[1]["value"], "user rendered by name")

		gateway, err := srv.GetObjectMetadata(pluginCtx, &pb.GetObjectMetadataRequest{ObjectType: concrete, ObjectId: "ast_1", Visibility: "gateway"})
		require.NoError(t, err)
		assert.JSONEq(t, `{"internal_note":"do not ship"}`, gateway.ValuesJson)
		assert.Empty(t, gateway.DisplayJson)

		_, err = srv.GetObjectMetadata(pluginCtx, &pb.GetObjectMetadataRequest{ObjectType: concrete, ObjectId: "ast_1", Visibility: "everyone"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))

		// A portal read of an object without a record is a clean "not found", not an error.
		missing, err := srv.GetObjectMetadata(pluginCtx, &pb.GetObjectMetadataRequest{ObjectType: self, ObjectId: "ast_404", Visibility: "portal"})
		require.NoError(t, err)
		assert.False(t, missing.Found)
		assert.Equal(t, "{}", missing.ValuesJson)
	})

	t.Run("delete removes the record and is idempotent", func(t *testing.T) {
		del, err := srv.DeleteObjectMetadata(pluginCtx, &pb.DeleteObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1"})
		require.NoError(t, err)
		assert.True(t, del.Success)
		_, err = gm.GetObjectMetadata(concrete, "ast_1")
		assert.Error(t, err)
		audits, _ := gm.ListAudit(concrete, "ast_1", 5)
		require.NotEmpty(t, audits)
		assert.Equal(t, "delete", audits[0].Action)
		assert.Equal(t, models.MetadataSourcePlugin(plugin.ID), audits[0].Source)

		again, err := srv.DeleteObjectMetadata(pluginCtx, &pb.DeleteObjectMetadataRequest{ObjectType: self, ObjectId: "ast_1"})
		require.NoError(t, err)
		assert.True(t, again.Success)

		_, err = srv.DeleteObjectMetadata(pluginCtx, &pb.DeleteObjectMetadataRequest{ObjectType: concrete})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestGovernedMetadataScopeMappingCoversDelete(t *testing.T) {
	assert.Equal(t, models.ServiceScopeMetadataWrite, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/DeleteObjectMetadata"))
}
