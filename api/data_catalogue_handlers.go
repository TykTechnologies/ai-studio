package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new data catalogue
// @Description Create a new data catalogue with the provided information
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param dataCatalogue body DataCatalogueInput true "Data Catalogue information"
// @Success 201 {object} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues [post]
// @Security BearerAuth
func (a *API) createDataCatalogue(c *gin.Context) {
	var input DataCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	dataCatalogue, err := a.service.CreateDataCatalogue(
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeDataCatalogue(dataCatalogue)})
}

// @Summary Get a data catalogue by ID
// @Description Get details of a data catalogue by its ID
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 200 {object} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /data-catalogues/{id} [get]
// @Security BearerAuth
func (a *API) getDataCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	dataCatalogue, err := a.service.GetDataCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Data catalogue not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogue(dataCatalogue)})
}

// @Summary Update a data catalogue
// @Description Update an existing data catalogue's information
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param dataCatalogue body DataCatalogueInput true "Updated data catalogue information"
// @Success 200 {object} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id} [patch]
// @Security BearerAuth
func (a *API) updateDataCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	var input DataCatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	dataCatalogue, err := a.service.UpdateDataCatalogue(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogue(dataCatalogue)})
}

// @Summary Delete a data catalogue
// @Description Delete a data catalogue by its ID
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id} [delete]
// @Security BearerAuth
func (a *API) deleteDataCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	err = a.service.DeleteDataCatalogue(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary List all data catalogues
// @Description Get a list of all data catalogues
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Success 200 {array} DataCatalogueResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues [get]
// @Security BearerAuth
func (a *API) listDataCatalogues(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	dataCatalogues, totalCount, totalPages, err := a.service.GetAllDataCatalogues(pageSize, pageNumber, all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Header("X-Total-Count", strconv.FormatInt(totalCount, 10))
	c.Header("X-Total-Pages", strconv.Itoa(totalPages))
	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogues(dataCatalogues)})
}

// @Summary Search data catalogues
// @Description Search for data catalogues using a query string
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param query query string true "Search query"
// @Success 200 {array} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/search [get]
// @Security BearerAuth
func (a *API) searchDataCatalogues(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Search query is required"}},
		})
		return
	}

	dataCatalogues, err := a.service.SearchDataCatalogues(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogues(dataCatalogues)})
}

// @Summary Add a tag to a data catalogue
// @Description Add a tag to a specific data catalogue
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param tag body DataCatalogueTagInput true "Tag to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id}/tags [post]
// @Security BearerAuth
func (a *API) addTagToDataCatalogue(c *gin.Context) {
	dataCatalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	var input DataCatalogueTagInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	tagID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tag ID"}},
		})
		return
	}

	err = a.service.AddTagToDataCatalogue(uint(dataCatalogueID), uint(tagID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a tag from a data catalogue
// @Description Remove a tag from a specific data catalogue
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param tagId path int true "Tag ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id}/tags/{tagId} [delete]
// @Security BearerAuth
func (a *API) removeTagFromDataCatalogue(c *gin.Context) {
	dataCatalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	tagID, err := strconv.ParseUint(c.Param("tagId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tag ID"}},
		})
		return
	}

	err = a.service.RemoveTagFromDataCatalogue(uint(dataCatalogueID), uint(tagID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Add a datasource to a data catalogue
// @Description Add a datasource to a specific data catalogue
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param datasource body DataCatalogueDatasourceInput true "Datasource to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id}/datasources [post]
// @Security BearerAuth
func (a *API) addDatasourceToDataCatalogue(c *gin.Context) {
	dataCatalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	var input DataCatalogueDatasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	datasourceID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	err = a.service.AddDatasourceToDataCatalogue(uint(dataCatalogueID), uint(datasourceID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Remove a datasource from a data catalogue
// @Description Remove a datasource from a specific data catalogue
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param id path int true "Data Catalogue ID"
// @Param datasourceId path int true "Datasource ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/{id}/datasources/{datasourceId} [delete]
// @Security BearerAuth
func (a *API) removeDatasourceFromDataCatalogue(c *gin.Context) {
	dataCatalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid data catalogue ID"}},
		})
		return
	}

	datasourceID, err := strconv.ParseUint(c.Param("datasourceId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	err = a.service.RemoveDatasourceFromDataCatalogue(uint(dataCatalogueID), uint(datasourceID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// @Summary Get data catalogues by tag
// @Description Get a list of data catalogues associated with a specific tag
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param tagName query string true "Tag name"
// @Success 200 {array} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/by-tag [get]
// @Security BearerAuth
func (a *API) getDataCataloguesByTag(c *gin.Context) {
	tagName := c.Query("tagName")
	if tagName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Tag name is required"}},
		})
		return
	}

	dataCatalogues, err := a.service.GetDataCataloguesByTag(tagName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogues(dataCatalogues)})
}

// @Summary Get data catalogues by datasource
// @Description Get a list of data catalogues associated with a specific datasource
// @Tags data-catalogues
// @Accept json
// @Produce json
// @Param datasourceId query int true "Datasource ID"
// @Success 200 {array} DataCatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /data-catalogues/by-datasource [get]
// @Security BearerAuth
func (a *API) getDataCataloguesByDatasource(c *gin.Context) {
	datasourceID, err := strconv.ParseUint(c.Query("datasourceId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	dataCatalogues, err := a.service.GetDataCataloguesByDatasource(uint(datasourceID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDataCatalogues(dataCatalogues)})
}

func serializeDataCatalogue(dataCatalogue *models.DataCatalogue) DataCatalogueResponse {
	return DataCatalogueResponse{
		Type: "data-catalogues",
		ID:   strconv.FormatUint(uint64(dataCatalogue.ID), 10),
		Attributes: struct {
			Name             string               `json:"name"`
			ShortDescription string               `json:"short_description"`
			LongDescription  string               `json:"long_description"`
			Icon             string               `json:"icon"`
			Datasources      []DatasourceResponse `json:"datasources"`
			Tags             []TagResponse        `json:"tags"`
		}{
			Name:             dataCatalogue.Name,
			ShortDescription: dataCatalogue.ShortDescription,
			LongDescription:  dataCatalogue.LongDescription,
			Icon:             dataCatalogue.Icon,
			Datasources:      serializeDatasources(dataCatalogue.Datasources),
			Tags:             serializeTags(dataCatalogue.Tags),
		},
	}
}

func serializeDataCatalogues(dataCatalogues models.DataCatalogues) []DataCatalogueResponse {
	result := make([]DataCatalogueResponse, len(dataCatalogues))
	for i, dataCatalogue := range dataCatalogues {
		// Serialize datasources inline to avoid N+1 queries
		datasourceResponses := make([]DatasourceResponse, len(dataCatalogue.Datasources))
		for j, datasource := range dataCatalogue.Datasources {
			datasourceResponses[j] = serializeDatasource(&datasource) // Note: this may still have N+1 if it accesses relationships
		}

		// Serialize tags inline to avoid N+1 queries
		tagResponses := make([]TagResponse, len(dataCatalogue.Tags))
		for j, tag := range dataCatalogue.Tags {
			tagResponses[j] = TagResponse{
				Type: "tags",
				ID:   strconv.FormatUint(uint64(tag.ID), 10),
				Attributes: struct {
					Name string `json:"name"`
				}{
					Name: tag.Name,
				},
			}
		}

		result[i] = DataCatalogueResponse{
			Type: "data-catalogues",
			ID:   strconv.FormatUint(uint64(dataCatalogue.ID), 10),
			Attributes: struct {
				Name             string               `json:"name"`
				ShortDescription string               `json:"short_description"`
				LongDescription  string               `json:"long_description"`
				Icon             string               `json:"icon"`
				Datasources      []DatasourceResponse `json:"datasources"`
				Tags             []TagResponse        `json:"tags"`
			}{
				Name:             dataCatalogue.Name,
				ShortDescription: dataCatalogue.ShortDescription,
				LongDescription:  dataCatalogue.LongDescription,
				Icon:             dataCatalogue.Icon,
				Datasources:      datasourceResponses,
				Tags:             tagResponses,
			},
		}
	}
	return result
}
