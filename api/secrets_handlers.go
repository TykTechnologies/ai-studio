package api

import (
	"errors"
	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"net/http"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

func checkSecretKey(c *gin.Context) bool {
	if !secrets.EncryptionKeyConfigured() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Service Unavailable", Detail: "Secrets functionality is disabled. TYK_AI_SECRET_KEY environment variable is not set."}},
		})
		return false
	}
	return true
}

// @Summary Create a new secret
// @Description Create a new secret with the provided information
// @Tags secrets
// @Accept json
// @Produce json
// @Param secret body SecretInput true "Secret information"
// @Success 201 {object} SecretResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Another secret has the name, or objects in another Workspace reference it"
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets [post]
// @Security BearerAuth
func (a *API) createSecret(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}
	var input SecretInput
	// Bound with the body kept, so workspace_id can be read from it too.
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		log.Printf("[DEBUG] Failed to bind JSON input: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	// The Workspace the secret lands in (decision D5).
	workspaceID, ok := a.resolveCreateFromBody(c, "secrets")
	if !ok {
		return
	}
	secret := &secrets.Secret{
		VarName:     input.Data.Attributes.VarName,
		Value:       input.Data.Attributes.Value,
		WorkspaceID: workspaceID,
	}

	log.Printf("[DEBUG] Creating secret with name: %s", secret.VarName)
	err := a.service.CreateSecret(secret)
	if isWorkspaceNotFound(err) {
		invalidWorkspace(c)
		return
	}
	if secretNameConflict(c, err) {
		return
	}
	if err != nil {
		log.Printf("[DEBUG] Failed to create secret: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	a.emitSecretChanged(c, services.TopicSecretCreated, "created", secret.ID)
	c.JSON(http.StatusCreated, gin.H{"data": serializeSecret(secret, a.secretReferencesFor([]string{secret.VarName}))})
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
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets/{id} [get]
// @Security BearerAuth
func (a *API) getSecret(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}
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

	secret, err := secrets.GetSecretByID(a.config.DB, uint(id), true) // Preserve reference format when viewing
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Secret not found"}},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": serializeSecret(secret, a.secretReferencesFor([]string{secret.VarName}))})
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
// @Failure 409 {object} ErrorResponse "Renaming a referenced secret, a name another secret has or objects in another Workspace reference, a concurrent move, or moving a duplicate name"
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets/{id} [patch]
// @Security BearerAuth
func (a *API) updateSecret(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}
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
	if err := c.ShouldBindBodyWith(&input, binding.JSON); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}

	secret, err := secrets.GetSecretByID(a.config.DB, uint(id), true) // Preserve reference format when editing
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Not Found", Detail: "Secret not found"}},
		})
		return
	}
	// Write in the secret's Workspace; a changed workspace_id is a move. A
	// referenced secret does not move: its references would then point
	// across Workspaces (.38).
	requested, ok := boundWorkspaceID(c)
	if !ok {
		return
	}
	moveTo, ok := a.checkObjectUpdate(c, "secrets", secret.WorkspaceID, requested)
	if !ok {
		return
	}
	if moveTo != nil {
		holders, err := a.service.SecretHoldersOf(secret.VarName)
		if err != nil {
			simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
			c.Abort()
			return
		}
		if len(holders) > 0 {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, authz.Body("Invalid Workspace",
				"the secret is referenced; change the references before moving it to another Workspace", authz.CodeInvalidWorkspace))
			return
		}
		// A grandfathered duplicate name stays put: moving it could make a
		// name that resolves today ambiguous, or the reverse.
		if secretNameConflict(c, a.service.RequireSecretMovable(secret)) {
			return
		}
	}
	oldName := secret.VarName

	// An omitted var_name keeps the stored name (96e); an empty one would
	// break every $SECRET/ reference to the secret, so it is refused.
	present, err := boundAttributeKeys(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Bad Request", Detail: err.Error()}},
		})
		return
	}
	if _, ok := present["var_name"]; ok {
		if strings.TrimSpace(input.Data.Attributes.VarName) == "" {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Errors: []struct {
					Title  string `json:"title"`
					Detail string `json:"detail"`
				}{{Title: "Bad Request", Detail: "var_name must not be empty"}},
			})
			return
		}
		secret.VarName = input.Data.Attributes.VarName
	}

	// Only update the value if the user actually provided a new secret value.
	// When editing, the frontend loads the placeholder reference (e.g. $SECRET/NAME)
	// and may send it back unchanged — we must not overwrite the real encrypted value.
	valueChanged := !secrets.IsSecretReference(input.Data.Attributes.Value) && input.Data.Attributes.Value != ""
	if valueChanged {
		secret.Value = input.Data.Attributes.Value
	}

	// The move goes first, so the stored Workspace matches the checks
	// made against the destination; a failed update puts it back.
	finishMove, ok := a.beginMove(c, &secrets.Secret{}, secret.ID, secret.WorkspaceID, moveTo)
	if !ok {
		return
	}
	updated := false
	defer func() { finishMove(updated) }()
	dest := secret.WorkspaceID
	if moveTo != nil {
		dest = *moveTo
	}
	if err := a.service.UpdateSecret(secret, oldName, dest, valueChanged); err != nil {
		if secretNameConflict(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}
	if moveTo != nil {
		secret.WorkspaceID = *moveTo
	}
	updated = true

	a.emitSecretChanged(c, services.TopicSecretUpdated, "updated", secret.ID)
	c.JSON(http.StatusOK, gin.H{"data": serializeSecret(secret, a.secretReferencesFor([]string{secret.VarName}))})
}

// @Summary Delete a secret
// @Description Delete a secret by its ID
// @Tags secrets
// @Accept json
// @Produce json
// @Param id path int true "Secret ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "The secret is referenced"
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets/{id} [delete]
// @Security BearerAuth
func (a *API) deleteSecret(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}
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

	if err := a.service.DeleteSecret(uint(id)); err != nil {
		if secretNameConflict(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Errors: []struct {
				Title  string `json:"title"`
				Detail string `json:"detail"`
			}{{Title: "Internal Server Error", Detail: err.Error()}},
		})
		return
	}

	a.emitSecretChanged(c, services.TopicSecretDeleted, "deleted", uint(id))
	c.Status(http.StatusNoContent)
}

// secretNameConflict answers 409 for the secret-name rules
// (services.ErrSecretNameTaken, ErrSecretDuplicateName,
// SecretReferencedError) and returns true; any other error (or nil) is
// left to the caller.
func secretNameConflict(c *gin.Context, err error) bool {
	var referenced *services.SecretReferencedError
	switch {
	case errors.Is(err, services.ErrSecretNameTaken), errors.Is(err, services.ErrSecretDuplicateName):
	case errors.As(err, &referenced):
	default:
		return false
	}
	simpleError(c, http.StatusConflict, "Conflict", err.Error())
	c.Abort()
	return true
}

// emitSecretChanged tells the control server a secret changed: the edges'
// snapshot carries referenced values (LLM keys, plugin config secrets), so
// they show as pending until the change is pushed. The event never carries
// the secret.
func (a *API) emitSecretChanged(c *gin.Context, topic, action string, id uint) {
	if a.service == nil || a.service.SystemEvents == nil {
		return
	}
	a.service.SystemEvents.EmitSecretChanged(topic, action, id, currentUserID(c))
}

// @Summary List all secrets
// @Description Get a paginated list of all secrets
// @Tags secrets
// @Accept json
// @Produce json
// @Param page_size query int false "Number of items per page"
// @Param page query int false "Page number"
// @Param all query bool false "Return all records without pagination"
// @Param search query string false "Case-insensitive substring match on name and description fields"
// @Param sort query string false "Sort field, prefix with - for descending. One of: id, name (var_name), created_at, updated_at"
// @Success 200 {object} SecretListResponse
// @Failure 500 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse "When TYK_AI_SECRET_KEY is not set"
// @Router /secrets [get]
// @Security BearerAuth
func (a *API) listSecrets(c *gin.Context) {
	if !checkSecretKey(c) {
		return
	}
	pageSize, pageNumber, all := getPaginationParams(c)
	opts, ok := parseListQuery(c, secretSortFields)
	if !ok {
		return
	}

	if opts.Filters, ok = a.listWorkspaceScopes(c, "secrets"); !ok {
		return
	}
	secrets, totalCount, totalPages, err := secrets.ListSecrets(a.config.DB, pageSize, pageNumber, all, opts.Scopes("var_name")...)
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
		Data: serializeSecrets(secrets, a.secretReferencesFor(secretVarNames(secrets))),
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

// secretReferences returns, keyed by secret name, the objects that read each
// secret. A failure to scan is logged and treated as "no references" so the
// secrets list still renders; the value is only advisory.
// secretReferencesFor resolves the objects that read each of the named
// secrets. Callers pass only the secrets they are about to render (the
// current page, or the single secret of a get/create/update), so the query
// against the secret_references index is bounded by that set. A lookup
// failure degrades to "no known consumers" rather than failing the request.
func (a *API) secretReferencesFor(names []string) map[string][]services.SecretReference {
	refs, err := a.service.SecretReferencesByNames(names)
	if err != nil {
		log.Warnf("Failed to resolve secret references: %v", err)
		return map[string][]services.SecretReference{}
	}
	return refs
}

// secretVarNames lists the names of the secrets on a page.
func secretVarNames(list []secrets.Secret) []string {
	names := make([]string, 0, len(list))
	for _, s := range list {
		names = append(names, s.VarName)
	}
	return names
}

func serializeSecret(secret *secrets.Secret, refs map[string][]services.SecretReference) SecretResponse {
	response := SecretResponse{
		Type: "secrets",
		ID:   strconv.FormatUint(uint64(secret.ID), 10),
	}
	response.Attributes.Value = secret.GetValue() // Use GetValue() to handle reference format
	response.Attributes.VarName = secret.VarName
	response.Attributes.HasValue = secret.HasValue()
	response.Attributes.WorkspaceID = secret.WorkspaceID
	response.Attributes.ReferencedBy = make([]SecretReferenceResponse, 0, len(refs[secret.VarName]))
	for _, ref := range refs[secret.VarName] {
		response.Attributes.ReferencedBy = append(response.Attributes.ReferencedBy, SecretReferenceResponse{
			Type: ref.Type,
			ID:   ref.ID,
			Name: ref.Name,
		})
	}
	return response
}

func serializeSecrets(secrets []secrets.Secret, refs map[string][]services.SecretReference) []SecretResponse {
	result := make([]SecretResponse, len(secrets))
	for i, secret := range secrets {
		result[i] = serializeSecret(&secret, refs)
	}
	return result
}
