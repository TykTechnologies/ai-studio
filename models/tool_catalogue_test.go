package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupToolCatalogueTest(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = InitModels(db)
	assert.NoError(t, err)

	return db
}

func createTestToolForCatalogue(t *testing.T, db *gorm.DB, name string) *Tool {
	tool := &Tool{
		Name:        name,
		Description: "Test Tool",
		ToolType:    "REST",
	}
	db.Create(tool)
	return tool
}

func createTestTagForCatalogue(t *testing.T, db *gorm.DB, name string) *Tag {
	tag := &Tag{Name: name}
	db.Create(tag)
	return tag
}

func TestNewToolCatalogue(t *testing.T) {
	t.Run("Create new tool catalogue", func(t *testing.T) {
		tc := NewToolCatalogue()
		assert.NotNil(t, tc)
		assert.Equal(t, uint(0), tc.ID)
	})
}

func TestToolCatalogue_Create(t *testing.T) {
	db := setupToolCatalogueTest(t)

	t.Run("Create tool catalogue successfully", func(t *testing.T) {
		tc := &ToolCatalogue{
			Name:             "Test Catalogue",
			ShortDescription: "Short desc",
			LongDescription:  "Long description",
		}

		err := tc.Create(db)
		assert.NoError(t, err)
		assert.NotZero(t, tc.ID)
	})
}

func TestToolCatalogue_Get(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc := &ToolCatalogue{Name: "Get Test"}
	db.Create(tc)

	t.Run("Get existing tool catalogue", func(t *testing.T) {
		retrieved := &ToolCatalogue{}
		err := retrieved.Get(db, tc.ID)
		assert.NoError(t, err)
		assert.Equal(t, "Get Test", retrieved.Name)
	})

	t.Run("Get non-existent tool catalogue", func(t *testing.T) {
		retrieved := &ToolCatalogue{}
		err := retrieved.Get(db, 99999)
		assert.Error(t, err)
	})
}

func TestToolCatalogue_Update(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc := &ToolCatalogue{Name: "Original"}
	db.Create(tc)

	t.Run("Update tool catalogue", func(t *testing.T) {
		tc.Name = "Updated"
		tc.ShortDescription = "Updated description"

		err := tc.Update(db)
		assert.NoError(t, err)

		retrieved := &ToolCatalogue{}
		retrieved.Get(db, tc.ID)
		assert.Equal(t, "Updated", retrieved.Name)
		assert.Equal(t, "Updated description", retrieved.ShortDescription)
	})
}

func TestToolCatalogue_Delete(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc := &ToolCatalogue{Name: "Delete Test"}
	db.Create(tc)

	t.Run("Delete tool catalogue", func(t *testing.T) {
		err := tc.Delete(db)
		assert.NoError(t, err)

		retrieved := &ToolCatalogue{}
		err = retrieved.Get(db, tc.ID)
		assert.Error(t, err)
	})
}

func TestToolCatalogue_TagOperations(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc := &ToolCatalogue{Name: "Tag Test"}
	db.Create(tc)

	tag1 := createTestTagForCatalogue(t, db, "Tag1")
	tag2 := createTestTagForCatalogue(t, db, "Tag2")

	t.Run("Add tag to tool catalogue", func(t *testing.T) {
		err := tc.AddTag(db, tag1)
		assert.NoError(t, err)

		tc.Get(db, tc.ID)
		assert.Len(t, tc.Tags, 1)
	})

	t.Run("Add multiple tags", func(t *testing.T) {
		err := tc.AddTag(db, tag2)
		assert.NoError(t, err)

		tc.Get(db, tc.ID)
		assert.Len(t, tc.Tags, 2)
	})

	t.Run("Remove tag from tool catalogue", func(t *testing.T) {
		err := tc.RemoveTag(db, tag1)
		assert.NoError(t, err)

		retrieved := &ToolCatalogue{}
		retrieved.Get(db, tc.ID)
		assert.Len(t, retrieved.Tags, 1)
		assert.Equal(t, tag2.ID, retrieved.Tags[0].ID)
	})
}

func TestToolCatalogue_ToolOperations(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc := &ToolCatalogue{Name: "Tool Test"}
	db.Create(tc)

	tool1 := createTestToolForCatalogue(t, db, "Tool1")
	tool2 := createTestToolForCatalogue(t, db, "Tool2")

	t.Run("Add tool to catalogue", func(t *testing.T) {
		err := tc.AddTool(db, tool1)
		assert.NoError(t, err)

		tc.Get(db, tc.ID)
		assert.Len(t, tc.Tools, 1)
	})

	t.Run("Add multiple tools", func(t *testing.T) {
		err := tc.AddTool(db, tool2)
		assert.NoError(t, err)

		tc.Get(db, tc.ID)
		assert.Len(t, tc.Tools, 2)
	})

	t.Run("Remove tool from catalogue", func(t *testing.T) {
		err := tc.RemoveTool(db, tool1)
		assert.NoError(t, err)

		retrieved := &ToolCatalogue{}
		retrieved.Get(db, tc.ID)
		assert.Len(t, retrieved.Tools, 1)
		assert.Equal(t, tool2.ID, retrieved.Tools[0].ID)
	})
}

func TestToolCatalogues_GetAll(t *testing.T) {
	db := setupToolCatalogueTest(t)

	for i := 1; i <= 7; i++ {
		tc := &ToolCatalogue{Name: "Catalogue " + string(rune('0'+i))}
		db.Create(tc)
	}

	t.Run("Get all with pagination", func(t *testing.T) {
		var catalogues ToolCatalogues
		totalCount, totalPages, err := catalogues.GetAll(db, 3, 1, false)
		assert.NoError(t, err)
		assert.Len(t, catalogues, 3)
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 3, totalPages)
	})

	t.Run("Get all without pagination", func(t *testing.T) {
		var catalogues ToolCatalogues
		totalCount, totalPages, err := catalogues.GetAll(db, 100, 1, true)
		assert.NoError(t, err)
		assert.Len(t, catalogues, 7)
		assert.Equal(t, int64(7), totalCount)
		assert.Equal(t, 1, totalPages)
	})
}

func TestToolCatalogues_Search(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc1 := &ToolCatalogue{Name: "Production Tools", ShortDescription: "Prod tools"}
	tc2 := &ToolCatalogue{Name: "Development Tools", ShortDescription: "Dev tools"}
	tc3 := &ToolCatalogue{Name: "Testing Utilities", LongDescription: "Testing tools for QA"}
	db.Create(tc1)
	db.Create(tc2)
	db.Create(tc3)

	t.Run("Search by name", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.Search(db, "Production")
		assert.NoError(t, err)
		assert.Len(t, catalogues, 1)
		assert.Equal(t, "Production Tools", catalogues[0].Name)
	})

	t.Run("Search by description", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.Search(db, "Testing")
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(catalogues), 1)
	})

	t.Run("Search with no matches", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.Search(db, "Nonexistent")
		assert.NoError(t, err)
		assert.Len(t, catalogues, 0)
	})
}

func TestToolCatalogues_GetByTag(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc1 := &ToolCatalogue{Name: "Tagged Catalogue 1"}
	tc2 := &ToolCatalogue{Name: "Tagged Catalogue 2"}
	tc3 := &ToolCatalogue{Name: "Untagged Catalogue"}
	db.Create(tc1)
	db.Create(tc2)
	db.Create(tc3)

	tag := createTestTagForCatalogue(t, db, "TestTag")
	tc1.AddTag(db, tag)
	tc2.AddTag(db, tag)

	t.Run("Get catalogues by tag", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.GetByTag(db, "TestTag")
		assert.NoError(t, err)
		assert.Len(t, catalogues, 2)
	})

	t.Run("Get by non-existent tag", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.GetByTag(db, "NonexistentTag")
		assert.NoError(t, err)
		assert.Len(t, catalogues, 0)
	})
}

func TestToolCatalogues_GetByTool(t *testing.T) {
	db := setupToolCatalogueTest(t)

	tc1 := &ToolCatalogue{Name: "Catalogue 1"}
	tc2 := &ToolCatalogue{Name: "Catalogue 2"}
	db.Create(tc1)
	db.Create(tc2)

	tool := createTestToolForCatalogue(t, db, "SharedTool")
	tc1.AddTool(db, tool)
	tc2.AddTool(db, tool)

	t.Run("Get catalogues by tool", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.GetByTool(db, tool.ID)
		assert.NoError(t, err)
		assert.Len(t, catalogues, 2)
	})

	t.Run("Get by non-existent tool", func(t *testing.T) {
		var catalogues ToolCatalogues
		err := catalogues.GetByTool(db, 99999)
		assert.NoError(t, err)
		assert.Len(t, catalogues, 0)
	})
}

func TestGetOrCreateDefaultToolCatalogue(t *testing.T) {
	db := setupToolCatalogueTest(t)

	t.Run("Create default catalogue when none exists", func(t *testing.T) {
		catalogue, err := GetOrCreateDefaultToolCatalogue(db)
		assert.NoError(t, err)
		assert.NotNil(t, catalogue)
		assert.Equal(t, "Default", catalogue.Name)
		assert.NotZero(t, catalogue.ID)
	})

	t.Run("Get existing default catalogue", func(t *testing.T) {
		first, _ := GetOrCreateDefaultToolCatalogue(db)
		second, err := GetOrCreateDefaultToolCatalogue(db)
		assert.NoError(t, err)
		assert.Equal(t, first.ID, second.ID)
	})
}

func TestToolCatalogue_IsDefault(t *testing.T) {
	t.Run("Default catalogue returns true", func(t *testing.T) {
		tc := &ToolCatalogue{Name: "Default"}
		assert.True(t, tc.IsDefault())
	})

	t.Run("Non-default catalogue returns false", func(t *testing.T) {
		tc := &ToolCatalogue{Name: "Custom"}
		assert.False(t, tc.IsDefault())
	})
}
