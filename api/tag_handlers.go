package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new tag
// @Description Create a new tag with the provided information
// @Tags tags
// @Accept json
// @Produce json
// @Param tag body TagInput true "Tag information"
// @Success 201 {object} TagResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tags [post]
// @Security BearerAuth
func (a *API) createTag(c *gin.Context) {
	var input TagInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	tag, err := a.service.CreateTag(input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeTag(tag)})
}

// @Summary Get a tag by ID
// @Description Get details of a tag by its ID
// @Tags tags
// @Accept json
// @Produce json
// @Param id path int true "Tag ID"
// @Success 200 {object} TagResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /tags/{id} [get]
// @Security BearerAuth
func (a *API) getTag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tag ID"}},
		})
		return
	}

	tag, err := a.service.GetTagByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Tag not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTag(tag)})
}

// @Summary Update a tag
// @Description Update an existing tag's information
// @Tags tags
// @Accept json
// @Produce json
// @Param id path int true "Tag ID"
// @Param tag body TagInput true "Updated tag information"
// @Success 200 {object} TagResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tags/{id} [patch]
// @Security BearerAuth
func (a *API) updateTag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tag ID"}},
		})
		return
	}

	var input TagInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	tag, err := a.service.UpdateTag(uint(id), input.Data.Attributes.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTag(tag)})
}

// @Summary Delete a tag
// @Description Delete a tag by its ID
// @Tags tags
// @Accept json
// @Produce json
// @Param id path int true "Tag ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tags/{id} [delete]
// @Security BearerAuth
func (a *API) deleteTag(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid tag ID"}},
		})
		return
	}

	err = a.service.DeleteTag(uint(id))
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

// @Summary List all tags
// @Description Get a list of all tags
// @Tags tags
// @Accept json
// @Produce json
// @Success 200 {array} TagResponse
// @Failure 500 {object} ErrorResponse
// @Router /tags [get]
// @Security BearerAuth
func (a *API) listTags(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	tags, totalCount, totalPages, err := a.service.GetAllTags(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeTags(tags)})
}

// @Summary Search tags by name
// @Description Search for tags using a name stub
// @Tags tags
// @Accept json
// @Produce json
// @Param name query string true "Name stub to search for"
// @Success 200 {array} TagResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /tags/search [get]
// @Security BearerAuth
func (a *API) searchTags(c *gin.Context) {
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

	tags, err := a.service.SearchTagsByNameStub(nameStub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeTags(tags)})
}

func serializeTag(tag *models.Tag) TagResponse {
	return TagResponse{
		Type: "tags",
		ID:   strconv.FormatUint(uint64(tag.ID), 10),
		Attributes: struct {
			Name string `json:"name"`
		}{
			Name: tag.Name,
		},
	}
}

func serializeTags(tags models.Tags) []TagResponse {
	result := make([]TagResponse, len(tags))
	for i, tag := range tags {
		result[i] = serializeTag(&tag)
	}
	return result
}
