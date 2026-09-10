//go:build enterprise
// +build enterprise

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

type denyHooks struct{}

func (denyHooks) RunMetadataHook(ctx context.Context, hookType string, rec *models.ObjectMetadata, userID uint) (*governed_metadata.HookOutcome, error) {
	if hookType == governed_metadata.HookBeforeUpdate {
		return &governed_metadata.HookOutcome{Allowed: false, RejectionReason: "denied by guard"}, nil
	}
	return &governed_metadata.HookOutcome{Allowed: true}, nil
}

func TestGovernedMetadataServer_PluginAttributionAndHookRejection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.LLM{}, &models.Plugin{}, &models.PluginResourceType{},
		&models.MetadataSchema{}, &models.MetadataVocabulary{}, &models.ObjectMetadata{}, &models.ObjectMetadataAudit{}))
	svc := &services.Service{DB: db, GovernedMetadataService: governed_metadata.NewService(db, governed_metadata.Deps{})}
	srv := NewGovernedMetadataServer(svc)
	require.NoError(t, svc.GovernedMetadata().CreateSchema(&models.MetadataSchema{Name: "S", Slug: "s", AppliesTo: []string{"llm"}, Active: true,
		Fields: []models.MetadataFieldDef{{Key: "k", Type: "string"}}}))

	// Writes made by an authenticated plugin are attributed to it.
	plugin := &models.Plugin{Name: "guard"}
	require.NoError(t, db.Create(plugin).Error)
	ctx := context.WithValue(context.Background(), pluginInfoKeyString, plugin)
	set, err := srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{"k":"v"}`})
	require.NoError(t, err)
	assert.True(t, set.Success)
	rec, err := svc.GovernedMetadata().GetObjectMetadata("llm", "1")
	require.NoError(t, err)
	assert.Equal(t, models.MetadataSourcePlugin(plugin.ID), rec.UpdatedBySource)
	audits, _ := svc.GovernedMetadata().ListAudit("llm", "1", 5)
	require.Len(t, audits, 1)
	assert.Equal(t, models.MetadataSourcePlugin(plugin.ID), audits[0].Source)

	// A hook rejection surfaces as PermissionDenied.
	svc.GovernedMetadataService = governed_metadata.NewService(db, governed_metadata.Deps{Hooks: denyHooks{}})
	_, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{"k":"w"}`, Merge: true})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "denied by guard")
	rec, _ = svc.GovernedMetadata().GetObjectMetadata("llm", "1")
	assert.Equal(t, "v", rec.Values["k"], "rejected write leaves the stored value")

	// Merge semantics through gRPC.
	svc.GovernedMetadataService = governed_metadata.NewService(db, governed_metadata.Deps{})
	set, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{}`, Merge: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"k":"v"}`, set.ValuesJson, "merging an empty object keeps existing keys")
	set, err = srv.SetObjectMetadata(ctx, &pb.SetObjectMetadataRequest{ObjectType: "llm", ObjectId: "1", ValuesJson: `{}`})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, set.ValuesJson, "replacing with an empty object clears")
}
