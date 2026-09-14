package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services/group_access"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The portal catalogue page must not show an LLM that is not live, even when
// it is in an accessible catalogue: GetAccessibleLLMs already filters on
// active, and the per-catalogue listing used to bypass that. The admin
// listing keeps showing everything in the catalogue.
func TestCommon_GetCatalogueLLMs_ExcludesInactive(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)

	user := createTestUser(t, api.service)
	catalogue := createTestCatalogue(t, api.service)
	addCatalogueToUserGroup(t, api.service, user.ID, catalogue.ID)

	live := createTestLLM(t, api.service, "Live LLM")
	draft, err := api.service.CreateLLM("Draft LLM", "api_key", "https://api.example.com",
		80, "Not yet approved", "Long desc", "", models.OPENAI, false, nil, "", []string{}, nil, nil, false, nil, nil)
	require.NoError(t, err)
	require.NoError(t, api.service.AddLLMToCatalogue(live.ID, catalogue.ID))
	require.NoError(t, api.service.AddLLMToCatalogue(draft.ID, catalogue.ID))

	t.Run("portal listing hides the inactive LLM", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", user)
		c.Params = []gin.Param{{Key: "id", Value: fmt.Sprintf("%d", catalogue.ID)}}

		api.getCatalogueLLMs(c)

		require.Equal(t, http.StatusOK, w.Code)
		var response []LLMResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.Len(t, response, 1)
		assert.Equal(t, "Live LLM", response[0].Attributes.Name)
	})

	t.Run("admin listing still shows the inactive LLM", func(t *testing.T) {
		w := performRequest(api.router, "GET", fmt.Sprintf("/api/v1/catalogues/%d/llms", catalogue.ID), nil)
		if !group_access.IsFilteringEnabled() {
			// Catalogue management is enterprise-only; CE answers 402 here.
			require.Equal(t, http.StatusPaymentRequired, w.Code)
			return
		}
		require.Equal(t, http.StatusOK, w.Code)

		var response map[string][]LLMResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		names := []string{}
		for _, item := range response["data"] {
			names = append(names, item.Attributes.Name)
		}
		assert.ElementsMatch(t, []string{"Live LLM", "Draft LLM"}, names)
	})
}
