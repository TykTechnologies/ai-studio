package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new datasource
// @Description Create a new datasource with the provided information
// @Tags datasources
// @Accept json
// @Produce json
// @Param datasource body DatasourceInput true "Datasource information"
// @Success 201 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources [post]
// @Security BearerAuth
func (a *API) createDatasource(c *gin.Context) {
	var input DatasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	datasource, err := a.service.CreateDatasource(
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
		input.Data.Attributes.Url,
		input.Data.Attributes.PrivacyScore,
		input.Data.Attributes.UserID,
		input.Data.Attributes.Tags,
		input.Data.Attributes.DBConnString,
		input.Data.Attributes.DBSourceType,
		input.Data.Attributes.DBConnAPIKey,
		input.Data.Attributes.DBName,
		input.Data.Attributes.EmbedVendor,
		input.Data.Attributes.EmbedUrl,
		input.Data.Attributes.EmbedAPIKey,
		input.Data.Attributes.EmbedModel,
		input.Data.Attributes.Active,
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeDatasource(datasource)})
}

// @Summary Get a datasource by ID
// @Description Get details of a datasource by its ID
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Success 200 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /datasources/{id} [get]
// @Security BearerAuth
func (a *API) getDatasource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	datasource, err := a.service.GetDatasourceByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Datasource not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDatasource(datasource)})
}

// @Summary Update a datasource
// @Description Update an existing datasource's information
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Param datasource body DatasourceInput true "Updated datasource information"
// @Success 200 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id} [patch]
// @Security BearerAuth
func (a *API) updateDatasource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	var input DatasourceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	datasource, err := a.service.UpdateDatasource(
		uint(id),
		input.Data.Attributes.Name,
		input.Data.Attributes.ShortDescription,
		input.Data.Attributes.LongDescription,
		input.Data.Attributes.Icon,
		input.Data.Attributes.Url,
		input.Data.Attributes.PrivacyScore,
		input.Data.Attributes.DBConnString,
		input.Data.Attributes.DBSourceType,
		input.Data.Attributes.DBConnAPIKey,
		input.Data.Attributes.DBName,
		input.Data.Attributes.EmbedVendor,
		input.Data.Attributes.EmbedUrl,
		input.Data.Attributes.EmbedAPIKey,
		input.Data.Attributes.EmbedModel,
		input.Data.Attributes.Active,
		input.Data.Attributes.Tags,
		input.Data.Attributes.UserID,
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

	c.JSON(http.StatusOK, gin.H{"data": serializeDatasource(datasource)})
}

// @Summary Delete a datasource
// @Description Delete a datasource by its ID
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id} [delete]
// @Security BearerAuth
func (a *API) deleteDatasource(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	err = a.service.DeleteDatasource(uint(id))
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

// @Summary List all datasources
// @Description Get a list of all datasources
// @Tags datasources
// @Accept json
// @Produce json
// @Success 200 {array} DatasourceResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources [get]
// @Security BearerAuth
func (a *API) listDatasources(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	datasources, totalCount, totalPages, err := a.service.GetAllDatasources(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeDatasources(datasources)})
}

// @Summary Search datasources
// @Description Search for datasources using a query string
// @Tags datasources
// @Accept json
// @Produce json
// @Param query query string true "Search query"
// @Success 200 {array} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/search [get]
// @Security BearerAuth
func (a *API) searchDatasources(c *gin.Context) {
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

	datasources, err := a.service.SearchDatasources(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDatasources(datasources)})
}

// @Summary Get datasources by tag
// @Description Get a list of datasources associated with a specific tag
// @Tags datasources
// @Accept json
// @Produce json
// @Param tag query string true "Tag name"
// @Success 200 {array} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/by-tag [get]
// @Security BearerAuth
func (a *API) getDatasourcesByTag(c *gin.Context) {
	tag := c.Query("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Tag name is required"}},
		})
		return
	}

	datasources, err := a.service.GetDatasourcesByTag(tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeDatasources(datasources)})
}

func serializeDatasource(datasource *models.Datasource) DatasourceResponse {
	return DatasourceResponse{
		Type: "datasources",
		ID:   strconv.FormatUint(uint64(datasource.ID), 10),
		Attributes: struct {
			Name             string        `json:"name"`
			ShortDescription string        `json:"short_description"`
			LongDescription  string        `json:"long_description"`
			Icon             string        `json:"icon"`
			Url              string        `json:"url"`
			PrivacyScore     int           `json:"privacy_score"`
			UserID           uint          `json:"user_id"`
			Tags             []TagResponse `json:"tags"`
			DBConnString     string        `json:"db_conn_string"`
			DBSourceType     string        `json:"db_source_type"`
			DBConnAPIKey     string        `json:"db_conn_api_key"`
			DBName           string        `json:"db_name"`
			EmbedVendor      string        `json:"embed_vendor"`
			EmbedUrl         string        `json:"embed_url"`
			EmbedAPIKey      string        `json:"embed_api_key"`
			EmbedModel       string        `json:"embed_model"`
			Active           bool          `json:"active"`
		}{
			Name:             datasource.Name,
			ShortDescription: datasource.ShortDescription,
			LongDescription:  datasource.LongDescription,
			Icon:             datasource.Icon,
			Url:              datasource.Url,
			PrivacyScore:     datasource.PrivacyScore,
			UserID:           datasource.UserID,
			Tags:             serializeTags(datasource.Tags),
			DBConnString:     datasource.DBConnString,
			DBSourceType:     datasource.DBSourceType,
			DBConnAPIKey:     datasource.DBConnAPIKey,
			DBName:           datasource.DBName,
			EmbedVendor:      string(datasource.EmbedVendor),
			EmbedUrl:         datasource.EmbedUrl,
			EmbedAPIKey:      datasource.EmbedAPIKey,
			EmbedModel:       datasource.EmbedModel,
			Active:           datasource.Active,
		},
	}
}

func serializeDatasources(datasources models.Datasources) []DatasourceResponse {
	result := make([]DatasourceResponse, len(datasources))
	for i, datasource := range datasources {
		result[i] = serializeDatasource(&datasource)
	}
	return result
}
