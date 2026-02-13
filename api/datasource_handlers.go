package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/TykTechnologies/midsommar/v2/data_session"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/services"
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

	// Namespace authorization: only admins can assign non-empty namespace
	if input.Data.Attributes.Namespace != "" {
		user, exists := c.Get("user")
		if !exists {
			c.JSON(http.StatusUnauthorized, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Unauthorized", Detail: "User not found in context"}}})
			return
		}
		if u, ok := user.(*models.User); !ok || !u.IsAdmin {
			c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can assign namespace"}}})
			return
		}
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
		input.Data.Attributes.Namespace,
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

	// Fetch existing datasource to check namespace change
	existingDS, err := a.service.GetDatasourceByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Not Found", Detail: "Datasource not found"}}})
		return
	}

	// Any namespace change (including clearing to global) requires admin authorization
	if input.Data.Attributes.Namespace != existingDS.Namespace {
		user, exists := c.Get("user")
		if !exists || func() bool { u, ok := user.(*models.User); return !ok || !u.IsAdmin }() {
			c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Forbidden", Detail: "Only administrators can change namespace"}}})
			return
		}
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
		input.Data.Attributes.Namespace,
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

// @Summary Add FileStore to Datasource
// @Description Add a FileStore to a specific Datasource
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Param filestore_id path int true "FileStore ID"
// @Success 200 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id}/filestores/{filestore_id} [post]
// @Security BearerAuth
func (a *API) addFileStoreToDatasource(c *gin.Context) {
	datasourceID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.AddFileToDatasource(uint(datasourceID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	datasource, err := a.service.GetDatasourceByID(uint(datasourceID))
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

// @Summary Remove FileStore from Datasource
// @Description Remove a FileStore from a specific Datasource
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Param filestore_id path int true "FileStore ID"
// @Success 200 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id}/filestores/{filestore_id} [delete]
// @Security BearerAuth
func (a *API) removeFileStoreFromDatasource(c *gin.Context) {
	datasourceID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid datasource ID"}},
		})
		return
	}

	fileStoreID, err := strconv.ParseUint(c.Param("filestore_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.RemoveFileFromDatasource(uint(datasourceID), uint(fileStoreID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	datasource, err := a.service.GetDatasourceByID(uint(datasourceID))
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

// @Summary Process file embeddings for a datasource
// @Description Process and create embeddings for all files in a datasource
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Datasource ID"
// @Success 202 {object} MessageResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id}/process-embeddings [post]
// @Security BearerAuth
func (a *API) ProcessFileEmbeddingHandler(c *gin.Context) {
	// Parse datasource ID from path
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

	// Get datasource to verify it exists
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

	// Initialize sources map for DataSession
	sources := make(map[uint]*models.Datasource)
	sources[datasource.ID] = datasource

	// Create new DataSession
	ds := data_session.NewDataSession(sources)

	// Process embeddings in a goroutine
	go func() {
		err := ds.ProcessRAGForDatasource(uint(id), a.service.DB)
		if err != nil {
			log.Printf("Error processing embeddings for datasource %d: %v", id, err)
			return
		}
		log.Printf("Successfully processed embeddings for datasource %d", id)

		// Update LastProcessedOn for all files in the datasource
		for _, file := range datasource.Files {
			file.LastProcessedOn = time.Now()
			err = file.Update(a.service.DB)
			if err != nil {
				log.Printf("Error updating LastProcessedOn for file %d: %v", file.ID, err)
			}
		}
	}()

	// Return accepted status immediately
	c.JSON(http.StatusAccepted, gin.H{
		"message": fmt.Sprintf("Processing embeddings for datasource %d started", id),
	})
}

// @Summary Clone a datasource
// @Description Creates a copy of an existing datasource including all API keys (server-side clone)
// @Tags datasources
// @Accept json
// @Produce json
// @Param id path int true "Source Datasource ID"
// @Success 201 {object} DatasourceResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /datasources/{id}/clone [post]
// @Security BearerAuth
func (a *API) cloneDatasource(c *gin.Context) {
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

	cloned, err := a.service.CloneDatasource(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeDatasource(cloned)})
}

func serializeDatasource(datasource *models.Datasource) DatasourceResponse {
	if datasource == nil {
		datasource = &models.Datasource{}
	}
	return DatasourceResponse{
		Type: "datasources",
		ID:   strconv.FormatUint(uint64(datasource.ID), 10),
		Attributes: struct {
			Name             string              `json:"name"`
			ShortDescription string              `json:"short_description"`
			LongDescription  string              `json:"long_description"`
			Icon             string              `json:"icon"`
			Url              string              `json:"url"`
			PrivacyScore     int                 `json:"privacy_score"`
			UserID           uint                `json:"user_id"`
			Tags             []TagResponse       `json:"tags"`
			DBConnString     string              `json:"db_conn_string"`
			DBSourceType     string              `json:"db_source_type"`
			DBConnAPIKey     string              `json:"db_conn_api_key"`
			HasDBConnAPIKey  bool                `json:"has_db_conn_api_key"`
			DBName           string              `json:"db_name"`
			EmbedVendor      string              `json:"embed_vendor"`
			EmbedUrl         string              `json:"embed_url"`
			EmbedAPIKey      string              `json:"embed_api_key"`
			HasEmbedAPIKey   bool                `json:"has_embed_api_key"`
			EmbedModel       string              `json:"embed_model"`
			Active           bool                `json:"active"`
			Namespace        string              `json:"namespace"`
			Files            []FileStoreResponse `json:"files"`
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
			DBConnAPIKey:     services.REDACTED_VALUE,
			HasDBConnAPIKey:  datasource.DBConnAPIKey != "",
			DBName:           datasource.DBName,
			EmbedVendor:      string(datasource.EmbedVendor),
			EmbedUrl:         datasource.EmbedUrl,
			EmbedAPIKey:      services.REDACTED_VALUE,
			HasEmbedAPIKey:   datasource.EmbedAPIKey != "",
			EmbedModel:       datasource.EmbedModel,
			Active:           datasource.Active,
			Namespace:        datasource.Namespace,
			Files:            serializeFileStores(datasource.Files),
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
