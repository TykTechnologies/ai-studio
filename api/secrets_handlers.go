package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new secret
// @Description Create a new secret with the provided information
// @Tags secrets
// @Accept json
// @Produce json
// @Param secret body SecretInput true "Secret information"
// @Success 201 {object} SecretResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets [post]
// @Security BearerAuth
func (a *API) createSecret(c *gin.Context) {
	var input SecretInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	secret := &secrets.Secret{
		VarName: input.Data.Attributes.VarName,
		Value:   input.Data.Attributes.Value,
	}

	if err := secrets.CreateSecret(a.config.DB, secret); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": serializeSecret(secret)})
}

// @Summary Get a secret by ID
// @Description Get details of a secret by its ID
// @Tags secrets
// @Accept json
// @Produce json
// @Param id path int true "Secret ID"
// @Success 200 {object} SecretResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /secrets/{id} [get]
// @Security BearerAuth
func (a *API) getSecret(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid secret ID"}},
		})
		return
	}

	secret, err := secrets.GetSecretByID(a.config.DB, uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Secret not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSecret(secret)})
}

// @Summary Update a secret
// @Description Update an existing secret's information
// @Tags secrets
// @Accept json
// @Produce json
// @Param id path int true "Secret ID"
// @Param secret body SecretInput true "Updated secret information"
// @Success 200 {object} SecretResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/{id} [patch]
// @Security BearerAuth
func (a *API) updateSecret(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid secret ID"}},
		})
		return
	}

	var input SecretInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	secret, err := secrets.GetSecretByID(a.config.DB, uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Secret not found"}},
		})
		return
	}

	secret.Value = input.Data.Attributes.Value
	secret.VarName = input.Data.Attributes.VarName

	if err := secrets.UpdateSecret(a.config.DB, secret); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSecret(secret)})
}

// @Summary Delete a secret
// @Description Delete a secret by its ID
// @Tags secrets
// @Accept json
// @Produce json
// @Param id path int true "Secret ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets/{id} [delete]
// @Security BearerAuth
func (a *API) deleteSecret(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: "Invalid secret ID"}},
		})
		return
	}

	if err := secrets.DeleteSecretByID(a.config.DB, uint(id)); err != nil {
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

// @Summary List all secrets
// @Description Get a paginated list of all secrets
// @Tags secrets
// @Accept json
// @Produce json
// @Param page_size query int false "Number of items per page"
// @Param page query int false "Page number"
// @Param all query bool false "Return all records without pagination"
// @Success 200 {object} SecretListResponse
// @Failure 500 {object} ErrorResponse
// @Router /secrets [get]
// @Security BearerAuth
func (a *API) listSecrets(c *gin.Context) {
	pageSize, pageNumber, all := getPaginationParams(c)

	secrets, totalCount, totalPages, err := secrets.ListSecrets(a.config.DB, pageSize, pageNumber, all)
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

	response := SecretListResponse{
		Data: serializeSecrets(secrets),
		Meta: struct {
			TotalCount int64 `json:"total_count"`
			TotalPages int   `json:"total_pages"`
			PageSize   int   `json:"page_size"`
			PageNumber int   `json:"page_number"`
		}{
			TotalCount: totalCount,
			TotalPages: totalPages,
			PageSize:   pageSize,
			PageNumber: pageNumber,
		},
	}

	c.JSON(http.StatusOK, response)
}

func serializeSecret(secret *secrets.Secret) SecretResponse {
	return SecretResponse{
		Type: "secrets",
		ID:   strconv.FormatUint(uint64(secret.ID), 10),
		Attributes: struct {
			Value   string `json:"value"`
			VarName string `json:"var_name"`
		}{
			Value:   secret.Value,
			VarName: secret.VarName,
		},
	}
}

func serializeSecrets(secrets []secrets.Secret) []SecretResponse {
	result := make([]SecretResponse, len(secrets))
	for i, secret := range secrets {
		result[i] = serializeSecret(&secret)
	}
	return result
}
