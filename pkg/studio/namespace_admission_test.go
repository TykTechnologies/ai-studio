package studio

import (
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/entitlements"
	"github.com/TykTechnologies/midsommar/v2/services/pushes"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A newly licensed namespace gets a configuration push, recorded for its
// edges; a namespace with no edge to push to is not an error.
func TestPushAdmittedNamespaces(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/p.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.EdgeInstance{}, &models.ClusterNode{}, &models.PushOperation{}, &models.EdgePushCommand{}))
	now := time.Now()
	require.NoError(t, db.Create(&models.EdgeInstance{EdgeID: "edge-a", Namespace: "team-a", Status: models.EdgeStatusConnected, LastHeartbeat: &now}).Error)

	c := pushes.New(db, "node-a", nil, pushes.Options{Entitlements: entitlements.NewCommunityService()})
	pushAdmittedNamespaces(c)([]string{"team-a", "team-empty"})

	var ops []models.PushOperation
	require.NoError(t, db.Find(&ops).Error)
	require.Len(t, ops, 1)
	assert.Equal(t, pushes.ScopeNamespace, ops[0].Scope)
	assert.Equal(t, "team-a", ops[0].Namespace)
	assert.Equal(t, admissionInitiator, ops[0].InitiatedBy)
	var cmds []models.EdgePushCommand
	require.NoError(t, db.Where("operation_id = ?", ops[0].OperationID).Find(&cmds).Error)
	require.Len(t, cmds, 1)
	assert.Equal(t, "edge-a", cmds[0].EdgeID)
}
