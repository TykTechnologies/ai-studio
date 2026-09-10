package grpc

import (
	"context"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	pb "github.com/TykTechnologies/midsommar/v2/proto/ai_studio_management"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/governed_metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newGovernedMetadataTestServer(t *testing.T) (*GovernedMetadataServer, *services.Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.LLM{}, &models.Plugin{}, &models.PluginResourceType{},
		&models.MetadataSchema{}, &models.MetadataVocabulary{}, &models.ObjectMetadata{}, &models.ObjectMetadataAudit{}))
	svc := &services.Service{DB: db, GovernedMetadataService: governed_metadata.NewService(db, governed_metadata.Deps{})}
	return NewGovernedMetadataServer(svc), svc
}

func TestGovernedMetadataScopeMapping(t *testing.T) {
	assert.Equal(t, models.ServiceScopeMetadataRead, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/GetObjectMetadata"))
	assert.Equal(t, models.ServiceScopeMetadataRead, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/GetResolvedMetadataSchema"))
	assert.Equal(t, models.ServiceScopeMetadataRead, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/ValidateObjectMetadata"))
	assert.Equal(t, models.ServiceScopeMetadataWrite, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/SetObjectMetadata"))
	// Pre-existing omission fixed alongside: PatchAppMetadata was unmapped and therefore uncallable.
	assert.Equal(t, models.ServiceScopeAppsWrite, extractScopeFromMethod("/ai_studio_management.AIStudioManagementService/PatchAppMetadata"))
}

func TestGovernedMetadataServer_ArgumentValidation(t *testing.T) {
	srv, _ := newGovernedMetadataTestServer(t)
	ctx := context.Background()

	_, err := srv.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "llm"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: "not json"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = srv.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = srv.ValidateObjectMetadata(ctx, &pb.ValidateObjectMetadataRequest{ObjectType: "llm", ValuesJson: "["})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// The behaviour of the read/write calls depends on the edition: CE reports
// nothing stored and refuses writes with FailedPrecondition; ENT round-trips.
func TestGovernedMetadataServer_EditionBehaviour(t *testing.T) {
	srv, svc := newGovernedMetadataTestServer(t)
	ctx := context.Background()

	get, err := srv.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
	require.NoError(t, err)
	assert.False(t, get.Found)
	assert.Equal(t, "{}", get.ValuesJson)

	resolved, err := srv.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "llm"})
	require.NoError(t, err)
	assert.Equal(t, "advisory", resolved.Enforcement)

	if !governed_metadata.IsEnterpriseAvailable() {
		_, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{"a":"b"}`})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		validate, err := srv.ValidateObjectMetadata(ctx, &pb.ValidateObjectMetadataRequest{ObjectType: "llm", ValuesJson: `{"a":"b"}`})
		require.NoError(t, err)
		assert.True(t, validate.Valid)
		return
	}

	gm := svc.GovernedMetadata()
	require.NoError(t, gm.CreateVocabulary(&models.MetadataVocabulary{Name: "Risk", Slug: "risk_tier", Terms: []models.VocabularyTerm{{Value: "high"}}}))
	require.NoError(t, gm.CreateSchema(&models.MetadataSchema{Name: "Core", Slug: "core", AppliesTo: []string{"llm"}, Active: true,
		Enforcement: models.MetadataEnforcementEnforce,
		Fields:      []models.MetadataFieldDef{{Key: "risk_tier", Type: "vocabulary", VocabularySlug: "risk_tier", Required: true}}}))

	// Enforced rejection is returned in-band so the plugin gets the structured result.
	set, err := srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{"risk_tier":"nonsense"}`})
	require.NoError(t, err)
	assert.False(t, set.Success)
	assert.Contains(t, set.ValidationResultJson, `"risk_tier"`)

	set, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{"risk_tier":"high"}`})
	require.NoError(t, err)
	assert.True(t, set.Success)
	assert.JSONEq(t, `{"risk_tier":"high"}`, set.ValuesJson)

	rec, err := gm.GetObjectMetadata("llm", "1")
	require.NoError(t, err)
	assert.Equal(t, models.MetadataSourceSystem, rec.UpdatedBySource, "no plugin in context → system source")

	get, err = srv.GetObjectMetadata(ctx, &pb.GetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1"})
	require.NoError(t, err)
	assert.True(t, get.Found)
	assert.Equal(t, "valid", get.ValidationStatus)
	assert.NotNil(t, get.UpdatedAt)

	validate, err := srv.ValidateObjectMetadata(ctx, &pb.ValidateObjectMetadataRequest{ObjectType: "llm", ValuesJson: `{}`})
	require.NoError(t, err)
	assert.False(t, validate.Valid)
	assert.True(t, validate.Enforced)

	resolved, err = srv.GetResolvedMetadataSchema(ctx, &pb.GetResolvedMetadataSchemaRequest{ObjectType: "llm"})
	require.NoError(t, err)
	assert.Equal(t, "enforce", resolved.Enforcement)
	assert.Equal(t, []string{"core"}, resolved.SchemaSlugs)
	assert.Contains(t, resolved.JsonSchema, `"additionalProperties":false`)

	_, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "spaceship", ObjectId: "1", ValuesJson: `{}`})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
