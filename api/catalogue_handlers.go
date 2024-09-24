package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new catalogue
// @Description Create a new catalogue with the provided information
// @Tags catalogues
// @Accept json
// @Produce json
// @Param catalogue body CatalogueInput true "Catalogue information"
// @Success 201 {object} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues [post]
// @Security BearerAuth
func (a *API) createCatalogue(c *gin.Context) {
	var input CatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	catalogue, err := a.service.CreateCatalogue(input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeCatalogue(catalogue)})
}

// @Summary Get a catalogue by ID
// @Description Get details of a catalogue by its ID
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {object} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /catalogues/{id} [get]
// @Security BearerAuth
func (a *API) getCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	catalogue, err := a.service.GetCatalogueByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Catalogue not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogue(catalogue)})
}

// @Summary Update a catalogue
// @Description Update an existing catalogue's information
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Param catalogue body CatalogueInput true "Updated catalogue information"
// @Success 200 {object} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/{id} [patch]
// @Security BearerAuth
func (a *API) updateCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	var input CatalogueInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	catalogue, err := a.service.UpdateCatalogue(uint(id), input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogue(catalogue)})
}

// @Summary Delete a catalogue
// @Description Delete a catalogue by its ID
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/{id} [delete]
// @Security BearerAuth
func (a *API) deleteCatalogue(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	err = a.service.DeleteCatalogue(uint(id))
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

// @Summary List all catalogues
// @Description Get a list of all catalogues with their associated LLM names
// @Tags catalogues
// @Accept json
// @Produce json
// @Success 200 {array} CatalogueResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues [get]
// @Security BearerAuth
func (a *API) listCatalogues(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	catalogues, totalCount, totalPages, err := a.service.GetAllCatalogues(pageSize, pageNumber, all)
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

	// Ensure LLMs are loaded for each catalogue
	for i := range catalogues {
		if err := catalogues[i].GetCatalogueLLMs(a.service.DB); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Internal Server Error", Detail: "Failed to load LLMs for catalogue"}},
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogues(catalogues)})
}

// @Summary Search catalogues by name
// @Description Search for catalogues using a name stub
// @Tags catalogues
// @Accept json
// @Produce json
// @Param name query string true "Name stub to search for"
// @Success 200 {array} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/search [get]
// @Security BearerAuth
func (a *API) searchCatalogues(c *gin.Context) {
	nameStub := c.Query("name")
	if nameStub == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name stub is required"}},
		})
		return
	}

	catalogues, err := a.service.SearchCataloguesByNameStub(nameStub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogues(catalogues)})
}

// @Summary Add an LLM to a catalogue
// @Description Add an LLM to a specific catalogue
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Param llm body CatalogueLLMInput true "LLM to add"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/{id}/llms [post]
// @Security BearerAuth
func (a *API) addLLMToCatalogue(c *gin.Context) {
	catalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	var input CatalogueLLMInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	llmID, err := strconv.ParseUint(input.Data.ID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	err = a.service.AddLLMToCatalogue(uint(llmID), uint(catalogueID))
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

// @Summary Remove an LLM from a catalogue
// @Description Remove an LLM from a specific catalogue
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Param llmId path int true "LLM ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/{id}/llms/{llmId} [delete]
// @Security BearerAuth
func (a *API) removeLLMFromCatalogue(c *gin.Context) {
	catalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	llmID, err := strconv.ParseUint(c.Param("llmId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid LLM ID"}},
		})
		return
	}

	err = a.service.RemoveLLMFromCatalogue(uint(llmID), uint(catalogueID))
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

// @Summary List LLMs in a catalogue
// @Description Get a list of all LLMs in a specific catalogue
// @Tags catalogues
// @Accept json
// @Produce json
// @Param id path int true "Catalogue ID"
// @Success 200 {array} LLMResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/{id}/llms [get]
// @Security BearerAuth
func (a *API) listCatalogueLLMs(c *gin.Context) {
	catalogueID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid catalogue ID"}},
		})
		return
	}

	llms, err := a.service.GetCatalogueLLMs(uint(catalogueID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeLLMs(llms)})
}

func serializeCatalogue(catalogue *models.Catalogue) CatalogueResponse {
	return CatalogueResponse{
		Type: "catalogues",
		ID:   strconv.FormatUint(uint64(catalogue.ID), 10),
		Attributes: struct {
			Name     string   `json:"name"`
			LLMNames []string `json:"llm_names"`
		}{
			Name:     catalogue.Name,
			LLMNames: catalogue.LLMNames(),
		},
	}
}

func serializeCatalogues(catalogues models.Catalogues) []CatalogueResponse {
	result := make([]CatalogueResponse, len(catalogues))
	for i, catalogue := range catalogues {
		result[i] = serializeCatalogue(&catalogue)
	}
	return result
}

// @Summary Search catalogues by name stub
// @Description Search for catalogues using a name stub
// @Tags catalogues
// @Accept json
// @Produce json
// @Param stub query string true "Name stub to search for"
// @Success 200 {array} CatalogueResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /catalogues/search-by-stub [get]
// @Security BearerAuth
func (a *API) searchCataloguesByNameStub(c *gin.Context) {
	stub := c.Query("stub")
	if stub == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Name stub is required"}},
		})
		return
	}

	catalogues, err := a.service.SearchCataloguesByNameStub(stub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeCatalogues(catalogues)})
}
