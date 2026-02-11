package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDBForAttestations(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestCreateAttestationTemplate(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	template, err := service.CreateAttestationTemplate(
		"Data Authority",
		"I confirm I have authority to share these credentials",
		models.AttestationAppliesToAll,
		true, true, 1,
	)
	assert.NoError(t, err)
	assert.NotNil(t, template)
	assert.NotZero(t, template.ID)
	assert.Equal(t, "Data Authority", template.Name)
	assert.True(t, template.Required)
	assert.True(t, template.Active)
}

func TestCreateAttestationTemplate_InvalidType(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	_, err := service.CreateAttestationTemplate(
		"Bad Template", "text", "invalid_type", true, true, 1,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid applies_to_type")
}

func TestUpdateAttestationTemplate(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	template, _ := service.CreateAttestationTemplate(
		"Original", "Original text", models.AttestationAppliesToAll, true, true, 1,
	)

	updated, err := service.UpdateAttestationTemplate(
		template.ID, "Updated", "Updated text", models.AttestationAppliesToDatasource, false, true, 2,
	)
	assert.NoError(t, err)
	assert.Equal(t, "Updated", updated.Name)
	assert.Equal(t, "Updated text", updated.Text)
	assert.Equal(t, models.AttestationAppliesToDatasource, updated.AppliesToType)
	assert.False(t, updated.Required)
}

func TestDeleteAttestationTemplate(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	template, _ := service.CreateAttestationTemplate(
		"To Delete", "text", models.AttestationAppliesToAll, true, true, 1,
	)

	err := service.DeleteAttestationTemplate(template.ID)
	assert.NoError(t, err)

	_, err = service.GetAttestationTemplateByID(template.ID)
	assert.Error(t, err)
}

func TestGetAllAttestationTemplates(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	service.CreateAttestationTemplate("Active 1", "text", models.AttestationAppliesToAll, true, true, 1)
	service.CreateAttestationTemplate("Active 2", "text", models.AttestationAppliesToDatasource, false, true, 2)
	service.CreateAttestationTemplate("Inactive", "text", models.AttestationAppliesToTool, true, false, 3)

	// All templates
	all, err := service.GetAllAttestationTemplates(false)
	assert.NoError(t, err)
	assert.Len(t, all, 3)

	// Active only
	active, err := service.GetAllAttestationTemplates(true)
	assert.NoError(t, err)
	assert.Len(t, active, 2)
}

func TestGetAttestationTemplatesByType(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	service.CreateAttestationTemplate("For All", "text", models.AttestationAppliesToAll, true, true, 1)
	service.CreateAttestationTemplate("For DS Only", "text", models.AttestationAppliesToDatasource, true, true, 2)
	service.CreateAttestationTemplate("For Tool Only", "text", models.AttestationAppliesToTool, true, true, 3)

	// Datasource templates should include "all" + "datasource"
	dsTemplates, err := service.GetAttestationTemplatesByType(models.AttestationAppliesToDatasource, true)
	assert.NoError(t, err)
	assert.Len(t, dsTemplates, 2)

	// Tool templates should include "all" + "tool"
	toolTemplates, err := service.GetAttestationTemplatesByType(models.AttestationAppliesToTool, true)
	assert.NoError(t, err)
	assert.Len(t, toolTemplates, 2)
}

func TestAttestationTemplates_SortOrder(t *testing.T) {
	db := setupTestDBForAttestations(t)
	service := NewService(db)

	service.CreateAttestationTemplate("Third", "text", models.AttestationAppliesToAll, true, true, 3)
	service.CreateAttestationTemplate("First", "text", models.AttestationAppliesToAll, true, true, 1)
	service.CreateAttestationTemplate("Second", "text", models.AttestationAppliesToAll, true, true, 2)

	templates, err := service.GetAllAttestationTemplates(false)
	assert.NoError(t, err)
	assert.Len(t, templates, 3)
	assert.Equal(t, "First", templates[0].Name)
	assert.Equal(t, "Second", templates[1].Name)
	assert.Equal(t, "Third", templates[2].Name)
}
