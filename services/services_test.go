package services

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	err = models.InitModels(db)
	assert.NoError(t, err)

	return db
}

func TestUserService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateUser
	user, err := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotZero(t, user.ID)

	// Test GetUserByID
	fetchedUser, err := service.GetUserByID(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.Email, fetchedUser.Email)

	// Test UpdateUser
	updatedUser, err := service.UpdateUser(user.ID, "updated@example.com", "Updated User", true, true, true, true)
	assert.NoError(t, err)
	assert.Equal(t, "updated@example.com", updatedUser.Email)
	assert.Equal(t, "Updated User", updatedUser.Name)

	// Test AuthenticateUser
	authenticatedUser, err := service.AuthenticateUser("updated@example.com", "password123")
	assert.NoError(t, err)
	assert.NotNil(t, authenticatedUser)

	// Test GetAllUsers
	users, _, _, err := service.GetAllUsers(10, 1, true, "id")
	assert.NoError(t, err)
	assert.Len(t, users, 1)

	// Test SearchUsersByEmailStub
	searchedUsers, err := service.SearchUsersByEmailStub("updat")
	assert.NoError(t, err)
	assert.Len(t, searchedUsers, 1)
	assert.Equal(t, "updated@example.com", searchedUsers[0].Email)

	// Test DeleteUser
	err = service.DeleteUser(user.ID)
	assert.NoError(t, err)

	// Verify user is deleted
	_, err = service.GetUserByID(user.ID)
	assert.Error(t, err)
}

func TestGroupService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateGroup
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)
	assert.NotNil(t, group)
	assert.NotZero(t, group.ID)

	// Test GetGroupByID
	fetchedGroup, err := service.GetGroupByID(group.ID)
	assert.NoError(t, err)
	assert.Equal(t, group.Name, fetchedGroup.Name)

	// Test UpdateGroup
	updatedGroup, err := service.UpdateGroup(group.ID, "Updated Group")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Group", updatedGroup.Name)

	// Test GetAllGroups
	groups, _, _, err := service.GetAllGroups(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, groups, 1)

	// Test SearchGroupsByNameStub
	searchedGroups, err := service.SearchGroupsByNameStub("Update")
	assert.NoError(t, err)
	assert.Len(t, searchedGroups, 1)
	assert.Equal(t, "Updated Group", searchedGroups[0].Name)

	// Test AddUserToGroup
	user, err := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true)
	assert.NoError(t, err)

	err = service.AddUserToGroup(user.ID, group.ID)
	assert.NoError(t, err)

	// Test GetGroupUsers
	groupUsers, err := service.GetGroupUsers(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupUsers, 1)
	assert.Equal(t, user.ID, groupUsers[0].ID)

	// Test RemoveUserFromGroup
	err = service.RemoveUserFromGroup(user.ID, group.ID)
	assert.NoError(t, err)

	groupUsers, err = service.GetGroupUsers(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupUsers, 0)

	// Test DeleteGroup
	err = service.DeleteGroup(group.ID)
	assert.NoError(t, err)

	// Verify group is deleted
	_, err = service.GetGroupByID(group.ID)
	assert.Error(t, err)
}

func TestLLMService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateLLM
	llm, err := service.CreateLLM("TestLLM", "test-api-key", "https://api.test.com", 75, "Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{})
	assert.NoError(t, err)
	assert.NotNil(t, llm)
	assert.NotZero(t, llm.ID)
	assert.Equal(t, "TestLLM", llm.Name)
	assert.Equal(t, "test-api-key", llm.APIKey)
	assert.Equal(t, "https://api.test.com", llm.APIEndpoint)
	assert.Equal(t, 75, llm.PrivacyScore)
	assert.Equal(t, "Short desc", llm.ShortDescription)
	assert.Equal(t, "Long desc", llm.LongDescription)
	assert.Equal(t, "https://logo.com", llm.LogoURL)

	// Test GetLLMByID
	fetchedLLM, err := service.GetLLMByID(llm.ID)
	assert.NoError(t, err)
	assert.Equal(t, llm.Name, fetchedLLM.Name)
	assert.Equal(t, llm.APIKey, fetchedLLM.APIKey)
	assert.Equal(t, llm.APIEndpoint, fetchedLLM.APIEndpoint)
	assert.Equal(t, llm.PrivacyScore, fetchedLLM.PrivacyScore)
	assert.Equal(t, llm.ShortDescription, fetchedLLM.ShortDescription)
	assert.Equal(t, llm.LongDescription, fetchedLLM.LongDescription)
	assert.Equal(t, llm.LogoURL, fetchedLLM.LogoURL)

	// Test UpdateLLM
	updatedLLM, err := service.UpdateLLM(llm.ID, "UpdatedLLM", "updated-api-key", "https://updated-api.test.com", 80,
		"Updated short", "Updated long", "https://updated-logo.com", models.OPENAI, true, nil, "", []string{})
	assert.NoError(t, err)
	assert.Equal(t, "UpdatedLLM", updatedLLM.Name)
	assert.Equal(t, "updated-api-key", updatedLLM.APIKey)
	assert.Equal(t, "https://updated-api.test.com", updatedLLM.APIEndpoint)
	assert.Equal(t, 80, updatedLLM.PrivacyScore)
	assert.Equal(t, "Updated short", updatedLLM.ShortDescription)
	assert.Equal(t, "Updated long", updatedLLM.LongDescription)
	assert.Equal(t, "https://updated-logo.com", updatedLLM.LogoURL)

	// Test GetLLMByName
	fetchedLLMByName, err := service.GetLLMByName("UpdatedLLM")
	assert.NoError(t, err)
	assert.Equal(t, updatedLLM.ID, fetchedLLMByName.ID)
	assert.Equal(t, updatedLLM.Name, fetchedLLMByName.Name)

	// Test GetAllLLMs
	allLLMs, _, _, err := service.GetAllLLMs(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allLLMs, 1)
	assert.Equal(t, updatedLLM.ID, (allLLMs)[0].ID)

	// Test GetLLMsByNameStub
	stubLLMs, err := service.GetLLMsByNameStub("Updated")
	assert.NoError(t, err)
	assert.Len(t, stubLLMs, 1)
	assert.Equal(t, updatedLLM.ID, (stubLLMs)[0].ID)

	// Test DeleteLLM
	err = service.DeleteLLM(llm.ID)
	assert.NoError(t, err)

	// Verify LLM is deleted
	_, err = service.GetLLMByID(llm.ID)
	assert.Error(t, err)

	// Test creating multiple LLMs and searching
	llm1, _ := service.CreateLLM("GPT-3", "key1", "https://api1.com", 70, "GPT-3 short", "GPT-3 long", "https://gpt3-logo.com", models.OPENAI, true, nil, "", []string{})
	llm2, _ := service.CreateLLM("GPT-4", "key2", "https://api2.com", 85, "GPT-4 short", "GPT-4 long", "https://gpt4-logo.com", models.OPENAI, true, nil, "", []string{})
	service.CreateLLM("BERT", "key3", "https://api3.com", 60, "BERT short", "BERT long", "https://bert-logo.com", models.OPENAI, true, nil, "", []string{})

	allLLMs, _, _, err = service.GetAllLLMs(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allLLMs, 3)

	gptLLMs, err := service.GetLLMsByNameStub("GPT")
	assert.NoError(t, err)
	assert.Len(t, gptLLMs, 2)
	assert.Contains(t, []uint{llm1.ID, llm2.ID}, (gptLLMs)[0].ID)
	assert.Contains(t, []uint{llm1.ID, llm2.ID}, (gptLLMs)[1].ID)
}

func createTestLLMs(t *testing.T, db *gorm.DB) {
	llms := []models.LLM{
		{Name: "LLM1", APIKey: "key1", APIEndpoint: "https://api1.com", PrivacyScore: 50, ShortDescription: "Short 1", LongDescription: "Long 1", LogoURL: "https://logo1.com"},
		{Name: "LLM2", APIKey: "key2", APIEndpoint: "https://api2.com", PrivacyScore: 75, ShortDescription: "Short 2", LongDescription: "Long 2", LogoURL: "https://logo2.com"},
		{Name: "LLM3", APIKey: "key3", APIEndpoint: "https://api3.com", PrivacyScore: 90, ShortDescription: "Short 3", LongDescription: "Long 3", LogoURL: "https://logo3.com"},
		{Name: "LLM4", APIKey: "key4", APIEndpoint: "https://api4.com", PrivacyScore: 30, ShortDescription: "Short 4", LongDescription: "Long 4", LogoURL: "https://logo4.com"},
		{Name: "LLM5", APIKey: "key5", APIEndpoint: "https://api5.com", PrivacyScore: 60, ShortDescription: "Short 5", LongDescription: "Long 5", LogoURL: "https://logo5.com"},
	}

	for _, llm := range llms {
		err := db.Create(&llm).Error
		assert.NoError(t, err)
	}
}

func TestService_GetLLMsByMaxPrivacyScore(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{100, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{80, 4, []string{"LLM1", "LLM2", "LLM4", "LLM5"}},
		{60, 3, []string{"LLM1", "LLM4", "LLM5"}},
		{40, 1, []string{"LLM4"}},
		{20, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByMaxPrivacyScore(tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}
}

func TestService_GetLLMsByMinPrivacyScore(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		minScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{40, 4, []string{"LLM1", "LLM2", "LLM3", "LLM5"}},
		{70, 2, []string{"LLM2", "LLM3"}},
		{80, 1, []string{"LLM3"}},
		{95, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByMinPrivacyScore(tc.minScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}
}

func TestService_GetLLMsByPrivacyScoreRange(t *testing.T) {
	db := setupTestDB(t)
	createTestLLMs(t, db)
	service := NewService(db)

	testCases := []struct {
		minScore      int
		maxScore      int
		expectedCount int
		expectedNames []string
	}{
		{0, 100, 5, []string{"LLM1", "LLM2", "LLM3", "LLM4", "LLM5"}},
		{40, 80, 3, []string{"LLM1", "LLM2", "LLM5"}},
		{70, 90, 2, []string{"LLM2", "LLM3"}},
		{30, 50, 2, []string{"LLM1", "LLM4"}},
		{95, 100, 0, []string{}},
	}

	for _, tc := range testCases {
		llms, err := service.GetLLMsByPrivacyScoreRange(tc.minScore, tc.maxScore)
		assert.NoError(t, err)
		assert.Len(t, llms, tc.expectedCount)

		var names []string
		for _, llm := range llms {
			names = append(names, llm.Name)
			assert.GreaterOrEqual(t, llm.PrivacyScore, tc.minScore)
			assert.LessOrEqual(t, llm.PrivacyScore, tc.maxScore)
		}
		assert.ElementsMatch(t, tc.expectedNames, names)
	}

	// Test invalid range
	llms, err := service.GetLLMsByPrivacyScoreRange(80, 70)
	assert.NoError(t, err)
	assert.Len(t, llms, 0)
}

func TestCatalogueService(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Test CreateCatalogue
	catalogue, err := service.CreateCatalogue("Test Catalogue")
	assert.NoError(t, err)
	assert.NotNil(t, catalogue)
	assert.NotZero(t, catalogue.ID)

	// Test GetCatalogueByID
	fetchedCatalogue, err := service.GetCatalogueByID(catalogue.ID)
	assert.NoError(t, err)
	assert.Equal(t, catalogue.Name, fetchedCatalogue.Name)

	// Test UpdateCatalogue
	updatedCatalogue, err := service.UpdateCatalogue(catalogue.ID, "Updated Catalogue")
	assert.NoError(t, err)
	assert.Equal(t, "Updated Catalogue", updatedCatalogue.Name)

	// Test GetAllCatalogues
	catalogues, _, _, err := service.GetAllCatalogues(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, catalogues, 1)

	// Test SearchCataloguesByNameStub
	searchedCatalogues, err := service.SearchCataloguesByNameStub("Update")
	assert.NoError(t, err)
	assert.Len(t, searchedCatalogues, 1)
	assert.Equal(t, "Updated Catalogue", searchedCatalogues[0].Name)

	// Test AddLLMToCatalogue
	llm, err := service.CreateLLM("TestLLM", "test-api-key", "https://api.test.com", 70, "Short desc", "Long desc", "https://logo.com", models.OPENAI, true, nil, "", []string{})
	assert.NoError(t, err)

	err = service.AddLLMToCatalogue(llm.ID, catalogue.ID)
	assert.NoError(t, err)

	// Test GetCatalogueLLMs
	catalogueLLMs, err := service.GetCatalogueLLMs(catalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 1)
	assert.Equal(t, llm.ID, catalogueLLMs[0].ID)

	// Test RemoveLLMFromCatalogue
	err = service.RemoveLLMFromCatalogue(llm.ID, catalogue.ID)
	assert.NoError(t, err)

	catalogueLLMs, err = service.GetCatalogueLLMs(catalogue.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 0)

	// Test DeleteCatalogue
	err = service.DeleteCatalogue(catalogue.ID)
	assert.NoError(t, err)

	// Verify catalogue is deleted
	_, err = service.GetCatalogueByID(catalogue.ID)
	assert.Error(t, err)
}

func TestCatalogueService_MultipleCatalogues(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create multiple catalogues
	catalogue1, _ := service.CreateCatalogue("AI Models")
	catalogue2, _ := service.CreateCatalogue("Machine Learning")
	catalogue3, _ := service.CreateCatalogue("Natural Language Processing")

	// Test GetAllCatalogues
	allCatalogues, _, _, err := service.GetAllCatalogues(10, 1, true)
	assert.NoError(t, err)
	assert.Len(t, allCatalogues, 3)

	// Test SearchCataloguesByNameStub
	aiCatalogues, err := service.SearchCataloguesByNameStub("AI")
	assert.NoError(t, err)
	assert.Len(t, aiCatalogues, 1)
	assert.Equal(t, catalogue1.ID, aiCatalogues[0].ID)

	mlCatalogues, err := service.SearchCataloguesByNameStub("Machine")
	assert.NoError(t, err)
	assert.Len(t, mlCatalogues, 1)
	assert.Equal(t, catalogue2.ID, mlCatalogues[0].ID)

	// Test adding multiple LLMs to a catalogue
	llm1, _ := service.CreateLLM("GPT-3", "key1", "https://api1.com", 80, "GPT-3 short", "GPT-3 long", "https://gpt3-logo.com", models.OPENAI, true, nil, "", []string{})
	llm2, _ := service.CreateLLM("BERT", "key2", "https://api2.com", 70, "BERT short", "BERT long", "https://bert-logo.com", models.OPENAI, true, nil, "", []string{})

	err = service.AddLLMToCatalogue(llm1.ID, catalogue3.ID)
	assert.NoError(t, err)
	err = service.AddLLMToCatalogue(llm2.ID, catalogue3.ID)
	assert.NoError(t, err)

	catalogueLLMs, err := service.GetCatalogueLLMs(catalogue3.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 2)
	assert.ElementsMatch(t, []uint{llm1.ID, llm2.ID}, []uint{catalogueLLMs[0].ID, catalogueLLMs[1].ID})

	// Test removing one LLM from the catalogue
	err = service.RemoveLLMFromCatalogue(llm1.ID, catalogue3.ID)
	assert.NoError(t, err)

	catalogueLLMs, err = service.GetCatalogueLLMs(catalogue3.ID)
	assert.NoError(t, err)
	assert.Len(t, catalogueLLMs, 1)
	assert.Equal(t, llm2.ID, catalogueLLMs[0].ID)
}

func TestUserAccessibleCatalogues(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create a user
	user, err := service.CreateUser("test@example.com", "Test User", "password123", true, true, true, true)
	assert.NoError(t, err)

	// Create groups
	group1, err := service.CreateGroup("Group 1")
	assert.NoError(t, err)
	group2, err := service.CreateGroup("Group 2")
	assert.NoError(t, err)

	// Add user to groups
	err = service.AddUserToGroup(user.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddUserToGroup(user.ID, group2.ID)
	assert.NoError(t, err)

	// Create catalogues
	catalogue1, err := service.CreateCatalogue("Catalogue 1")
	assert.NoError(t, err)
	catalogue2, err := service.CreateCatalogue("Catalogue 2")
	assert.NoError(t, err)
	_, err = service.CreateCatalogue("Catalogue 3")
	assert.NoError(t, err)

	// Add catalogues to groups
	err = service.AddCatalogueToGroup(catalogue1.ID, group1.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogue2.ID, group2.ID)
	assert.NoError(t, err)

	// Test GetUserAccessibleCatalogues
	accessibleCatalogues, err := service.GetUserAccessibleCatalogues(user.ID)
	assert.NoError(t, err)
	assert.Len(t, accessibleCatalogues, 2)
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, []string{accessibleCatalogues[0].Name, accessibleCatalogues[1].Name})

	// Remove user from a group
	err = service.RemoveUserFromGroup(user.ID, group2.ID)
	assert.NoError(t, err)

	// Test GetUserAccessibleCatalogues after removal
	accessibleCatalogues, err = service.GetUserAccessibleCatalogues(user.ID)
	assert.NoError(t, err)
	assert.Len(t, accessibleCatalogues, 1)
	assert.Equal(t, "Catalogue 1", accessibleCatalogues[0].Name)
}

func TestGroupCatalogueAssociation(t *testing.T) {
	db := setupTestDB(t)
	service := NewService(db)

	// Create a group
	group, err := service.CreateGroup("Test Group")
	assert.NoError(t, err)

	// Create catalogues
	catalogue1, err := service.CreateCatalogue("Catalogue 1")
	assert.NoError(t, err)
	catalogue2, err := service.CreateCatalogue("Catalogue 2")
	assert.NoError(t, err)

	// Test AddCatalogueToGroup
	err = service.AddCatalogueToGroup(catalogue1.ID, group.ID)
	assert.NoError(t, err)
	err = service.AddCatalogueToGroup(catalogue2.ID, group.ID)
	assert.NoError(t, err)

	// Test GetGroupCatalogues
	groupCatalogues, err := service.GetGroupCatalogues(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupCatalogues, 2)
	assert.ElementsMatch(t, []string{"Catalogue 1", "Catalogue 2"}, []string{groupCatalogues[0].Name, groupCatalogues[1].Name})

	// Test RemoveCatalogueFromGroup
	err = service.RemoveCatalogueFromGroup(catalogue1.ID, group.ID)
	assert.NoError(t, err)

	// Test GetGroupCatalogues after removal
	groupCatalogues, err = service.GetGroupCatalogues(group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupCatalogues, 1)
	assert.Equal(t, "Catalogue 2", groupCatalogues[0].Name)
}
