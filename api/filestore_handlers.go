package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/filereader"
	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new filestore entry
// @Description Create a new filestore entry with an uploaded file
// @Tags filestore
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "File to upload"
// @Param description formData string false "Description of the file"
// @Success 201 {object} FileStoreResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filestore [post]
// @Security BearerAuth
func (a *API) createFileStore(c *gin.Context) {
	// Get the file from the request
	file, fileHeader, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Error getting file from request: " + err.Error()}},
		})
		return
	}
	defer file.Close()

	// Check file size (optional - adjust limit as needed)
	const maxSize = 10 << 20 // 10 MB
	if fileHeader.Size > maxSize {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "File size exceeds maximum limit"}},
		})
		return
	}

	// Read the file contents
	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: "Error reading file: " + err.Error()}},
		})
		return
	}

	// Get the description from form data (optional)
	description := c.PostForm("description")

	// Handle file types
	textContent, err := filereader.Read(fileHeader.Filename, content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Error parsing file", Detail: err.Error()}}})
		return
	}

	// Create the filestore entry
	fileStore, err := a.service.CreateFileStore(
		fileHeader.Filename,
		description,
		string(base64.StdEncoding.EncodeToString([]byte(textContent))),
		len(content),
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

	c.JSON(http.StatusCreated, gin.H{"data": serializeFileStore(fileStore)})
}

// @Summary Get a filestore entry by ID
// @Description Get details of a filestore entry by its ID
// @Tags filestore
// @Accept json
// @Produce json
// @Param id path int true "FileStore ID"
// @Success 200 {object} FileStoreResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /filestore/{id} [get]
// @Security BearerAuth
func (a *API) getFileStore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	fileStore, err := a.service.GetFileStoreByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "FileStore entry not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeFileStore(fileStore)})
}

// @Summary Update a filestore entry
// @Description Update an existing filestore entry's information
// @Tags filestore
// @Accept json
// @Produce json
// @Param id path int true "FileStore ID"
// @Param filestore body FileStoreInput true "Updated filestore information"
// @Success 200 {object} FileStoreResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filestore/{id} [patch]
// @Security BearerAuth
func (a *API) updateFileStore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	var input FileStoreInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	fileStore, err := a.service.UpdateFileStore(
		uint(id),
		input.Data.Attributes.FileName,
		input.Data.Attributes.Description,
		input.Data.Attributes.Content,
		input.Data.Attributes.Length,
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

	c.JSON(http.StatusOK, gin.H{"data": serializeFileStore(fileStore)})
}

// @Summary Delete a filestore entry
// @Description Delete a filestore entry by its ID
// @Tags filestore
// @Accept json
// @Produce json
// @Param id path int true "FileStore ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filestore/{id} [delete]
// @Security BearerAuth
func (a *API) deleteFileStore(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid filestore ID"}},
		})
		return
	}

	err = a.service.DeleteFileStore(uint(id))
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

// @Summary Get all filestore entries
// @Description Get a list of all filestore entries with pagination
// @Tags filestore
// @Accept json
// @Produce json
// @Success 200 {array} FileStoreResponse
// @Failure 500 {object} ErrorResponse
// @Router /filestore [get]
// @Security BearerAuth
func (a *API) getAllFileStores(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	fileStores, totalCount, totalPages, err := a.service.GetAllFileStores(pageSize, pageNumber, all)
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
	c.JSON(http.StatusOK, gin.H{"data": serializeFileStores(fileStores)})
}

// @Summary Search filestore entries
// @Description Search for filestore entries by filename or description
// @Tags filestore
// @Accept json
// @Produce json
// @Param query query string true "Search Query"
// @Success 200 {array} FileStoreResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /filestore/search [get]
// @Security BearerAuth
func (a *API) searchFileStores(c *gin.Context) {
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

	fileStores, err := a.service.SearchFileStores(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeFileStores(fileStores)})
}

func serializeFileStore(fileStore *models.FileStore) FileStoreResponse {
	return FileStoreResponse{
		Type: "filestore",
		ID:   strconv.FormatUint(uint64(fileStore.ID), 10),
		Attributes: struct {
			FileName    string `json:"file_name"`
			Description string `json:"description"`
			Content     string `json:"-"`
			Length      int    `json:"length"`
		}{
			FileName:    fileStore.FileName,
			Description: fileStore.Description,
			Content:     fileStore.Content,
			Length:      fileStore.Length,
		},
	}
}

func serializeFileStores(fileStores []models.FileStore) []FileStoreResponse {
	result := make([]FileStoreResponse, len(fileStores))
	for i, fileStore := range fileStores {
		result[i] = serializeFileStore(&fileStore)
	}
	return result
}
