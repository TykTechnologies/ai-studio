package api

import (
	"net/http"
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/gin-gonic/gin"
)

// @Summary Create a new credential
// @Description Create a new credential
// @Tags credentials
// @Accept json
// @Produce json
// @Success 201 {object} CredentialResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials [post]
// @Security BearerAuth
func (a *API) createCredential(c *gin.Context) {
    credential, err := a.service.CreateCredential()
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    c.JSON(http.StatusCreated, gin.H{"data": serializeCredential(credential)})
}

// @Summary Get a credential by ID
// @Description Get details of a credential by its ID
// @Tags credentials
// @Accept json
// @Produce json
// @Param id path int true "Credential ID"
// @Success 200 {object} CredentialResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /credentials/{id} [get]
// @Security BearerAuth
func (a *API) getCredential(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: "Invalid credential ID"}},
        })
        return
    }

    credential, err := a.service.GetCredentialByID(uint(id))
    if err != nil {
        c.JSON(http.StatusNotFound, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Not Found", Detail: "Credential not found"}},
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{"data": serializeCredential(credential)})
}

// @Summary Get a credential by Key ID
// @Description Get details of a credential by its Key ID
// @Tags credentials
// @Accept json
// @Produce json
// @Param keyId path string true "Credential Key ID"
// @Success 200 {object} CredentialResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /credentials/key/{keyId} [get]
// @Security BearerAuth
func (a *API) getCredentialByKeyID(c *gin.Context) {
    keyID := c.Param("keyId")

    credential, err := a.service.GetCredentialByKeyID(keyID)
    if err != nil {
        c.JSON(http.StatusNotFound, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Not Found", Detail: "Credential not found"}},
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{"data": serializeCredential(credential)})
}

// @Summary Update a credential
// @Description Update an existing credential's information
// @Tags credentials
// @Accept json
// @Produce json
// @Param id path int true "Credential ID"
// @Param credential body CredentialInput true "Updated credential information"
// @Success 200 {object} CredentialResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials/{id} [patch]
// @Security BearerAuth
func (a *API) updateCredential(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: "Invalid credential ID"}},
        })
        return
    }

    var input CredentialInput
    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: err.Error()}},
        })
        return
    }

    credential, err := a.service.GetCredentialByID(uint(id))
    if err != nil {
        c.JSON(http.StatusNotFound, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Not Found", Detail: "Credential not found"}},
        })
        return
    }

    credential.Active = input.Data.Attributes.Active

    err = a.service.UpdateCredential(credential)
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{"data": serializeCredential(credential)})
}

// @Summary Delete a credential
// @Description Delete a credential by its ID
// @Tags credentials
// @Accept json
// @Produce json
// @Param id path int true "Credential ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials/{id} [delete]
// @Security BearerAuth
func (a *API) deleteCredential(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: "Invalid credential ID"}},
        })
        return
    }

    err = a.service.DeleteCredential(uint(id))
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

// @Summary Activate a credential
// @Description Activate a credential by its ID
// @Tags credentials
// @Accept json
// @Produce json
// @Param id path int true "Credential ID"
// @Success 200 {object} CredentialResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials/{id}/activate [post]
// @Security BearerAuth
func (a *API) activateCredential(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: "Invalid credential ID"}},
        })
        return
    }

    err = a.service.ActivateCredential(uint(id))
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    credential, _ := a.service.GetCredentialByID(uint(id))
    c.JSON(http.StatusOK, gin.H{"data": serializeCredential(credential)})
}

// @Summary Deactivate a credential
// @Description Deactivate a credential by its ID
// @Tags credentials
// @Accept json
// @Produce json
// @Param id path int true "Credential ID"
// @Success 200 {object} CredentialResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials/{id}/deactivate [post]
// @Security BearerAuth
func (a *API) deactivateCredential(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 32)
    if err != nil {
        c.JSON(http.StatusBadRequest, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Bad Request", Detail: "Invalid credential ID"}},
        })
        return
    }

    err = a.service.DeactivateCredential(uint(id))
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    credential, _ := a.service.GetCredentialByID(uint(id))
    c.JSON(http.StatusOK, gin.H{"data": serializeCredential(credential)})
}

// @Summary List all credentials
// @Description Get a list of all credentials
// @Tags credentials
// @Accept json
// @Produce json
// @Success 200 {array} CredentialResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials [get]
// @Security BearerAuth
func (a *API) listCredentials(c *gin.Context) {
    credentials, err := a.service.GetAllCredentials()
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{"data": serializeCredentials(credentials)})
}

// @Summary List active credentials
// @Description Get a list of all active credentials
// @Tags credentials
// @Accept json
// @Produce json
// @Success 200 {array} CredentialResponse
// @Failure 500 {object} ErrorResponse
// @Router /credentials/active [get]
// @Security BearerAuth
func (a *API) listActiveCredentials(c *gin.Context) {
    credentials, err := a.service.GetActiveCredentials()
    if err != nil {
        c.JSON(http.StatusInternalServerError, ErrorResponse{
            Errors: []struct {
                Title  string `json:"title"`
                Detail string `json:"detail"`
            }{{Title: "Internal Server Error", Detail: err.Error()}},
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{"data": serializeCredentials(credentials)})
}

func serializeCredential(credential *models.Credential) CredentialResponse {
    return CredentialResponse{
        Type: "credentials",
        ID:   strconv.FormatUint(uint64(credential.ID), 10),
        Attributes: struct {
            KeyID  string `json:"key_id"`
            Secret string `json:"secret"`
            Active bool   `json:"active"`
        }{
            KeyID:  credential.KeyID,
            Secret: credential.Secret,
            Active: credential.Active,
        },
    }
}

func serializeCredentials(credentials models.Credentials) []CredentialResponse {
    result := make([]CredentialResponse, len(credentials))
    for i, credential := range credentials {
        result[i] = serializeCredential(&credential)
    }
    return result
}
