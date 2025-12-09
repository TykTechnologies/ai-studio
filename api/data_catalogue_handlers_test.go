//go:build enterprise
// +build enterprise

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataCatalogueEndpoints(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Test Create DataCatalogue
	createDataCatalogueInput := DataCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "data-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name:             "Test Data Catalogue",
				ShortDescription: "Short description",
				LongDescription:  "Long description",
				Icon:             "icon.png",
			},
		},
	}

	w := performRequest(api.router, "POST", "/api/v1/data-catalogues", createDataCatalogueInput)
	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]DataCatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "Test Data Catalogue", response["data"].Attributes.Name)

	dataCatalogueID := response["data"].ID

	// Test Get DataCatalogue
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/%s", dataCatalogueID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Update DataCatalogue
	updateDataCatalogueInput := DataCatalogueInput{
		Data: struct {
			Type       string `json:"type"`
			Attributes struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			} `json:"attributes"`
		}{
			Type: "data-catalogues",
			Attributes: struct {
				Name             string `json:"name"`
				ShortDescription string `json:"short_description"`
				LongDescription  string `json:"long_description"`
				Icon             string `json:"icon"`
			}{
				Name:             "Updated Data Catalogue",
				ShortDescription: "Updated short description",
				LongDescription:  "Updated long description",
				Icon:             "updated-icon.png",
			},
		},
	}

	w = performRequest(api.router, "PATCH", fmt.Sprintf("/api/v1/data-catalogues/%s", dataCatalogueID), updateDataCatalogueInput)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test List DataCatalogues
	w = performRequest(api.router, "GET", "/api/v1/data-catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Test Search DataCatalogues
	w = performRequest(api.router, "GET", "/api/v1/data-catalogues/search?query=Updated", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 1)
	assert.Equal(t, "Updated Data Catalogue", searchResponse["data"][0].Attributes.Name)

	// Test Add Tag to DataCatalogue
	tag, err := api.service.CreateTag("Test Tag")
	assert.NoError(t, err)

	addTagInput := DataCatalogueTagInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "tags",
			ID:   fmt.Sprintf("%d", tag.ID),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/data-catalogues/%s/tags", dataCatalogueID), addTagInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Get DataCatalogues by Tag
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-tag?tagName=%s", tag.Name), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var tagResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &tagResponse)
	assert.NoError(t, err)
	assert.Len(t, tagResponse["data"], 1)
	assert.Equal(t, "Updated Data Catalogue", tagResponse["data"][0].Attributes.Name)

	// Test Remove Tag from DataCatalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/data-catalogues/%s/tags/%d", dataCatalogueID, tag.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Add Datasource to DataCatalogue
	datasource, err := api.service.CreateDatasource(
		"Test Datasource",
		"Short Desc",
		"Long Desc",
		"icon.png",
		"https://example.com",
		75,
		1,
		[]string{},
		"conn_string",
		"source_type",
		"api_key",
		"db1",
		"embed_vendor",
		"embed_url",
		"embed_api_key",
		"embed_model",
		true,
	)
	assert.NoError(t, err)

	addDatasourceInput := DataCatalogueDatasourceInput{
		Data: struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}{
			Type: "datasources",
			ID:   fmt.Sprintf("%d", datasource.ID),
		},
	}

	w = performRequest(api.router, "POST", fmt.Sprintf("/api/v1/data-catalogues/%s/datasources", dataCatalogueID), addDatasourceInput)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Get DataCatalogues by Datasource
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-datasource?datasourceId=%d", datasource.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var datasourceResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &datasourceResponse)
	assert.NoError(t, err)
	// Note: Datasource is auto-assigned to "Default" catalogue when created
	assert.Len(t, datasourceResponse["data"], 2) // Test catalogue + Default catalogue
	// Verify our updated catalogue is in the results
	var foundUpdated bool
	for _, dc := range datasourceResponse["data"] {
		if dc.Attributes.Name == "Updated Data Catalogue" {
			foundUpdated = true
			break
		}
	}
	assert.True(t, foundUpdated, "Updated Data Catalogue should be in datasource's catalogues")

	// Test Remove Datasource from DataCatalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/data-catalogues/%s/datasources/%d", dataCatalogueID, datasource.ID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Test Delete DataCatalogue
	w = performRequest(api.router, "DELETE", fmt.Sprintf("/api/v1/data-catalogues/%s", dataCatalogueID), nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify data catalogue is deleted
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/%s", dataCatalogueID), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDataCatalogueEndpoints_MultipleDataCatalogues(t *testing.T) {
	api, _ := setupTestAPI(t)

	// Create multiple data catalogues
	createDataCatalogue := func(name string) string {
		input := DataCatalogueInput{
			Data: struct {
				Type       string `json:"type"`
				Attributes struct {
					Name             string `json:"name"`
					ShortDescription string `json:"short_description"`
					LongDescription  string `json:"long_description"`
					Icon             string `json:"icon"`
				} `json:"attributes"`
			}{
				Type: "data-catalogues",
				Attributes: struct {
					Name             string `json:"name"`
					ShortDescription string `json:"short_description"`
					LongDescription  string `json:"long_description"`
					Icon             string `json:"icon"`
				}{
					Name:             name,
					ShortDescription: "Short description",
					LongDescription:  "Long description",
					Icon:             "icon.png",
				},
			},
		}

		w := performRequest(api.router, "POST", "/api/v1/data-catalogues", input)
		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]DataCatalogueResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		return response["data"].ID
	}

	dc1ID := createDataCatalogue("Data Catalogue 1")
	dc2ID := createDataCatalogue("Data Catalogue 2")
	dc3ID := createDataCatalogue("Data Catalogue 3")

	// Test List All Data Catalogues
	w := performRequest(api.router, "GET", "/api/v1/data-catalogues", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResponse map[string][]DataCatalogueResponse
	err := json.Unmarshal(w.Body.Bytes(), &listResponse)
	assert.NoError(t, err)
	assert.Len(t, listResponse["data"], 3)

	// Test Search Data Catalogues
	w = performRequest(api.router, "GET", "/api/v1/data-catalogues/search?query=Catalogue", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var searchResponse map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &searchResponse)
	assert.NoError(t, err)
	assert.Len(t, searchResponse["data"], 3)

	// Create tags and add to data catalogues
	tag1, _ := api.service.CreateTag("Tag 1")
	tag2, _ := api.service.CreateTag("Tag 2")

	addTagToDataCatalogue := func(dcID string, tagID uint) {
		input := DataCatalogueTagInput{
			Data: struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}{
				Type: "tags",
				ID:   fmt.Sprintf("%d", tagID),
			},
		}

		w := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/data-catalogues/%s/tags", dcID), input)
		assert.Equal(t, http.StatusNoContent, w.Code)
	}

	addTagToDataCatalogue(dc1ID, tag1.ID)
	addTagToDataCatalogue(dc2ID, tag1.ID)
	addTagToDataCatalogue(dc2ID, tag2.ID)
	addTagToDataCatalogue(dc3ID, tag2.ID)

	// Test Get Data Catalogues by Tag
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-tag?tagName=%s", tag1.Name), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var tag1Response map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &tag1Response)
	assert.NoError(t, err)
	assert.Len(t, tag1Response["data"], 2)

	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-tag?tagName=%s", tag2.Name), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var tag2Response map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &tag2Response)
	assert.NoError(t, err)
	assert.Len(t, tag2Response["data"], 2)

	// Create datasources and add to data catalogues
	ds1, _ := api.service.CreateDatasource(
		"Datasource 1",
		"Short 1",
		"Long 1",
		"icon1.png",
		"https://example1.com",
		75,
		1,
		[]string{},
		"conn_string1",
		"source_type1",
		"api_key1",
		"db1",
		"embed_vendor1",
		"embed_url1",
		"embed_api_key1",
		"embed_model1",
		true,
	)
	ds2, _ := api.service.CreateDatasource(
		"Datasource 2",
		"Short 2",
		"Long 2",
		"icon2.png",
		"https://example2.com",
		80,
		1,
		[]string{},
		"conn_string2",
		"source_type2",
		"api_key2",
		"db2",
		"embed_vendor2",
		"embed_url2",
		"embed_api_key2",
		"embed_model2",
		true,
	)

	addDatasourceToDataCatalogue := func(dcID string, dsID uint) {
		input := DataCatalogueDatasourceInput{
			Data: struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}{
				Type: "datasources",
				ID:   fmt.Sprintf("%d", dsID),
			},
		}

		w := performRequest(api.router, "POST", fmt.Sprintf("/api/v1/data-catalogues/%s/datasources", dcID), input)
		assert.Equal(t, http.StatusNoContent, w.Code)
	}

	addDatasourceToDataCatalogue(dc1ID, ds1.ID)
	addDatasourceToDataCatalogue(dc2ID, ds1.ID)
	addDatasourceToDataCatalogue(dc2ID, ds2.ID)

	// Test Get Data Catalogues by Datasource
	// Note: Datasources are auto-assigned to "Default" catalogue when created
	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-datasource?datasourceId=%d", ds1.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var ds1Response map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &ds1Response)
	assert.NoError(t, err)
	assert.Len(t, ds1Response["data"], 3) // dc1 + dc2 + Default

	w = performRequest(api.router, "GET", fmt.Sprintf("/api/v1/data-catalogues/by-datasource?datasourceId=%d", ds2.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var ds2Response map[string][]DataCatalogueResponse
	err = json.Unmarshal(w.Body.Bytes(), &ds2Response)
	assert.NoError(t, err)
	assert.Len(t, ds2Response["data"], 2) // dc2 + Default
}
