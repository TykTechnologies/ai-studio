package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/models"
	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/TykTechnologies/midsommar/v2/services/semantic_router"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Semantic Routers (Enterprise): routers that pick one of their named routes
// from what the prompt says. See pkg/semanticrouting for the configuration
// and docs/site/docs/semantic-router.md for the feature.

// SemanticRouterAttributes is a router as the API reads and writes it.
type SemanticRouterAttributes struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	// Active is the live switch. Omitted = inactive on create, unchanged on
	// update. Setting it needs semantic-routers:publish.
	Active    *bool  `json:"active"`
	Namespace string `json:"namespace"`
	// Portal presentation (the router is published in LLM catalogues).
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	LogoURL          string `json:"logo_url"`

	Settings sr.Settings `json:"settings"`
	Routes   []sr.Route  `json:"routes"`

	// EmbedderID is the embedder of the embedding stage (0 or omitted: none).
	// A settings.embedding naming an LLM ({llm_id, model}) is still accepted
	// and saved as the embedder linked to that LLM with that model.
	EmbedderID *uint `json:"embedder_id"`
}

// SemanticRouterInput is the JSON:API body of a create or update.
type SemanticRouterInput struct {
	Data struct {
		Type       string                   `json:"type"`
		Attributes SemanticRouterAttributes `json:"attributes"`
	} `json:"data"`
}

// SemanticRouterTestMessage is one message of a test conversation.
type SemanticRouterTestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SemanticRouterTestInput is what the test panel classifies. Model is what
// follows the slug in the model string: "auto" (the default) or a route name
// when explicit routes are allowed.
type SemanticRouterTestInput struct {
	Messages []SemanticRouterTestMessage `json:"messages" binding:"required"`
	Model    string                      `json:"model"`
	// Router is the draft to test (POST /semantic-routers/test only).
	Router *SemanticRouterAttributes `json:"router,omitempty"`
}

// semanticRouterSortFields are the columns ?sort accepts.
var semanticRouterSortFields = sortable("id", "name", "created_at", "updated_at", "active")

func (a *API) semanticRouters() semantic_router.Service {
	if a.service.SemanticRouterService != nil {
		return a.service.SemanticRouterService
	}
	return semantic_router.NewService(a.service.DB)
}

func attributesToSemanticRouter(in *SemanticRouterAttributes, activeIfOmitted bool) *models.SemanticRouter {
	active := activeIfOmitted
	if in.Active != nil {
		active = *in.Active
	}
	return &models.SemanticRouter{
		Name:             strings.TrimSpace(in.Name),
		Slug:             strings.TrimSpace(in.Slug),
		Description:      in.Description,
		Active:           active,
		Namespace:        in.Namespace,
		ShortDescription: in.ShortDescription,
		LongDescription:  in.LongDescription,
		LogoURL:          in.LogoURL,
		Settings:         in.Settings,
		Routes:           in.Routes,
		EmbedderID:       nonZero(in.EmbedderID),
	}
}

func nonZero(id *uint) *uint {
	if id == nil || *id == 0 {
		return nil
	}
	return id
}

// linkLegacyEmbedding saves an embedding stage given the old way (an LLM and
// model in settings.embedding) as the embedder linked to that LLM with that
// model, found or created, keeping only the timeout in settings. The router
// is validated first, so an invalid router (or the Community Edition) never
// creates an embedder as a side effect.
func (a *API) linkLegacyEmbedding(c *gin.Context, router *models.SemanticRouter) error {
	ref := router.Settings.Embedding
	if router.EmbedderID != nil || ref == nil || ref.LLMID == 0 {
		return nil
	}
	if err := a.semanticRouters().ValidateRouter(router); err != nil {
		return err
	}
	e, err := a.service.FindOrCreateLinkedEmbedder(ref.LLMID, ref.Model, currentUserID(c))
	if err != nil {
		return err
	}
	id := e.ID
	router.EmbedderID = &id
	router.Embedder = e
	if ref.TimeoutMs > 0 {
		router.Settings.Embedding = &sr.ModelRef{TimeoutMs: ref.TimeoutMs}
	} else {
		router.Settings.Embedding = nil
	}
	return nil
}

func parseSemanticRouterID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "Invalid semantic router ID")
		return 0, false
	}
	return uint(id), true
}

// @Summary Create a semantic router
// @Description Create a router that picks a route (an LLM and model, or a Model Router alias) from what the prompt says (Enterprise only)
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param router body SemanticRouterInput true "Semantic router"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 500 {object} ErrorResponse
// @Router /semantic-routers [post]
// @Security BearerAuth
func (a *API) createSemanticRouter(c *gin.Context) {
	var input SemanticRouterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	router := attributesToSemanticRouter(&input.Data.Attributes, false)
	if err := models.CheckLogoURL(router.LogoURL); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	// Creating a router already active is the publish action.
	if !a.requirePublishToCreateLive(c, "semantic-routers", router.Active) {
		return
	}
	if err := a.linkLegacyEmbedding(c, router); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if err := a.semanticRouters().CreateRouter(router); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitSemanticRouterCreated(router, router.ID, 0)
	}
	c.JSON(http.StatusCreated, gin.H{"data": serializeSemanticRouter(router)})
}

// @Summary Get a semantic router
// @Tags semantic-routers
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id} [get]
// @Security BearerAuth
func (a *API) getSemanticRouter(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	router, err := a.semanticRouters().GetRouter(id)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeSemanticRouter(router)})
}

// @Summary Update a semantic router
// @Description Replaces the router's configuration and presentation. Its catalogues are set with PUT /semantic-routers/{id}/catalogues.
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Param router body SemanticRouterInput true "Semantic router"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id} [patch]
// @Security BearerAuth
func (a *API) updateSemanticRouter(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	var input SemanticRouterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	existing, err := a.semanticRouters().GetRouter(id)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	router := attributesToSemanticRouter(&input.Data.Attributes, existing.Active)
	router.ID = id
	if err := models.CheckLogoURL(router.LogoURL); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	// Changing the live switch through an update is the publish action.
	if !a.requirePublishIfChanged(c, "semantic-routers", existing.Active, router.Active) {
		return
	}
	if err := a.linkLegacyEmbedding(c, router); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if err := a.semanticRouters().UpdateRouter(router); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	updated, err := a.semanticRouters().GetRouter(id)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitSemanticRouterUpdated(updated, id, 0)
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeSemanticRouter(updated)})
}

// @Summary Delete a semantic router
// @Description Deletes the router and withdraws it from every App and catalogue.
// @Tags semantic-routers
// @Param id path int true "Semantic Router ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id} [delete]
// @Security BearerAuth
func (a *API) deleteSemanticRouter(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	if err := a.semanticRouters().DeleteRouter(id); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitSemanticRouterDeleted(id, 0)
	}
	c.Status(http.StatusNoContent)
}

// @Summary List semantic routers
// @Tags semantic-routers
// @Produce json
// @Param page_size query int false "Number of items per page"
// @Param page query int false "Page number"
// @Param all query bool false "Return all items without pagination"
// @Param search query string false "Case-insensitive substring match on name and description"
// @Param sort query string false "Sort field, prefix with - for descending. One of: id, name, created_at, updated_at, active"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse "Unsupported sort field"
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Router /semantic-routers [get]
// @Security BearerAuth
func (a *API) listSemanticRouters(c *gin.Context) {
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	pageNumber, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	all := c.Query("all") == "true"
	opts, ok := parseListQuery(c, semanticRouterSortFields)
	if !ok {
		return
	}
	routers, total, pages, err := a.service.ListSemanticRouters(pageSize, pageNumber, all, opts)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	out := make([]map[string]interface{}, len(routers))
	for i := range routers {
		out[i] = serializeSemanticRouter(&routers[i])
	}
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{"total_count": total, "total_pages": pages, "page_size": pageSize, "page_number": pageNumber},
	})
}

// @Summary Publish or withdraw a semantic router
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Param active body object true "Active status"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id}/toggle [patch]
// @Security BearerAuth
func (a *API) toggleSemanticRouterActive(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	var input struct {
		Active *bool `json:"active" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if err := a.semanticRouters().ToggleRouterActive(id, *input.Active); err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	router, err := a.semanticRouters().GetRouter(id)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	if a.service.SystemEvents != nil {
		a.service.SystemEvents.EmitSemanticRouterUpdated(router, id, 0)
	}
	c.JSON(http.StatusOK, gin.H{"data": serializeSemanticRouter(router)})
}

// @Summary Publish a semantic router in LLM catalogues
// @Description Replaces the LLM catalogues the router is published in (Enterprise). Teams granted one of them see the router in the portal and may add it to their Apps.
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Param body body modelRouterCataloguesInput true "Catalogue ids"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id}/catalogues [put]
// @Security BearerAuth
func (a *API) setSemanticRouterCatalogues(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	if !semantic_router.IsEnterpriseAvailable() {
		respondSemanticRouterError(c, semantic_router.ErrEnterpriseFeature)
		return
	}
	var in modelRouterCataloguesInput
	if err := c.ShouldBindJSON(&in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	cats, err := a.service.SetSemanticRouterCatalogues(id, in.CatalogueIDs)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", "Semantic router not found")
		return
	case errors.Is(err, services.ErrInvalidCatalogueReference):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	case err != nil:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"catalogues": routerCatalogueRefs(cats)}})
}

// @Summary Get semantic router dependents
// @Description List the apps granted the router and the LLM catalogues it is published in
// @Tags semantic-routers
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Success 200 {object} DependentsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /semantic-routers/{id}/dependents [get]
// @Security BearerAuth
func (a *API) getSemanticRouterDependents(c *gin.Context) {
	a.respondDependents(c, dependentsLookup{
		exists: func(id uint) (bool, error) { return a.rowExists(&models.SemanticRouter{}, id) },
		fetch:  a.service.GetSemanticRouterDependents,
	})
}

// @Summary Test a saved semantic router
// @Description Classifies a conversation with the router's saved configuration, as the gateway would, and returns the decision with a per-stage trace. Embedding and judge calls go to the configured LLMs. The router need not be active.
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param id path int true "Semantic Router ID"
// @Param body body SemanticRouterTestInput true "Conversation to classify"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 404 {object} ErrorResponse
// @Failure 429 {object} ErrorResponse "More than 30 test runs a minute"
// @Router /semantic-routers/{id}/test [post]
// @Security BearerAuth
func (a *API) testSemanticRouter(c *gin.Context) {
	id, ok := parseSemanticRouterID(c)
	if !ok {
		return
	}
	var input SemanticRouterTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	router, err := a.semanticRouters().GetRouter(id)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	a.runSemanticRouterTest(c, router, &input)
}

// @Summary Test a draft semantic router
// @Description Classifies a conversation with an unsaved router configuration (the editor's test panel).
// @Tags semantic-routers
// @Accept json
// @Produce json
// @Param body body SemanticRouterTestInput true "Draft router and conversation"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 402 {object} ErrorResponse "Enterprise feature required"
// @Failure 429 {object} ErrorResponse "More than 30 test runs a minute"
// @Router /semantic-routers/test [post]
// @Security BearerAuth
func (a *API) testDraftSemanticRouter(c *gin.Context) {
	var input SemanticRouterTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}
	if input.Router == nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "router is required")
		return
	}
	a.runSemanticRouterTest(c, attributesToSemanticRouter(input.Router, false), &input)
}

// maxTestMessages bounds a test conversation.
const maxTestMessages = 50

func (a *API) runSemanticRouterTest(c *gin.Context, router *models.SemanticRouter, input *SemanticRouterTestInput) {
	actorID, _ := adminAppActor(c)
	if ok, wait := semanticRouterTests.allow(strconv.FormatUint(uint64(actorID), 10)); !ok {
		c.Header("Retry-After", retryAfterSeconds(wait))
		simpleError(c, http.StatusTooManyRequests, "Too Many Requests",
			"too many semantic router tests; the test panel is limited to 30 runs a minute per user")
		return
	}
	if len(input.Messages) == 0 || len(input.Messages) > maxTestMessages {
		simpleError(c, http.StatusBadRequest, "Bad Request", "messages must hold between 1 and 50 messages")
		return
	}
	req := sr.Request{Model: strings.TrimSpace(input.Model)}
	for _, m := range input.Messages {
		req.Messages = append(req.Messages, sr.Message{Role: m.Role, Content: m.Content})
	}
	decision, err := a.semanticRouters().Test(c.Request.Context(), router, req)
	if err != nil {
		respondSemanticRouterError(c, err)
		return
	}
	resp := gin.H{"decision": decision}
	if rt, ok := findRoute(router.Routes, decision.Route); ok {
		resp["target"] = rt.Target
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func findRoute(routes []sr.Route, name string) (sr.Route, bool) {
	for _, r := range routes {
		if r.Name == name {
			return r, true
		}
	}
	return sr.Route{}, false
}

// serializeSemanticRouter is a router in JSON:API form. Models are the
// model strings a client sends to the unified ingress for it.
func serializeSemanticRouter(r *models.SemanticRouter) map[string]interface{} {
	routes := r.Routes
	if routes == nil {
		routes = []sr.Route{}
	}
	return map[string]interface{}{
		"type": "semantic-routers",
		"id":   strconv.FormatUint(uint64(r.ID), 10),
		"attributes": map[string]interface{}{
			"name":              r.Name,
			"slug":              r.Slug,
			"description":       r.Description,
			"active":            r.Active,
			"namespace":         r.Namespace,
			"short_description": r.ShortDescription,
			"long_description":  r.LongDescription,
			"logo_url":          r.LogoURL,
			"settings":          settingsWithEmbedder(r),
			"embedder_id":       r.EmbedderID,
			"embedder_name":     routerEmbedderName(r),
			"routes":            routes,
			"models":            sr.ModelsFor(r.Slug, r.Config()),
			"catalogues":        routerCatalogueRefs(r.Catalogues),
			"created_at":        r.CreatedAt,
			"updated_at":        r.UpdatedAt,
		},
	}
}

// settingsWithEmbedder is the router's settings as the API shows them: the
// embedder is flattened into settings.embedding ({llm_id, model} for a linked
// embedder, {model} for a standalone one) so clients reading the old shape
// keep working.
func settingsWithEmbedder(r *models.SemanticRouter) sr.Settings {
	s := r.Settings
	if r.Embedder == nil {
		return s
	}
	ref := sr.ModelRef{Model: r.Embedder.ModelName}
	if s.Embedding != nil {
		ref.TimeoutMs = s.Embedding.TimeoutMs
	}
	if r.Embedder.IsLinked() {
		ref.LLMID = *r.Embedder.LLMID
	}
	s.Embedding = &ref
	return s
}

func routerEmbedderName(r *models.SemanticRouter) string {
	if r.Embedder == nil {
		return ""
	}
	return r.Embedder.Name
}

// respondSemanticRouterError maps a Semantic Router service error to its
// response: 402 in the Community Edition, 404 for a router that does not
// exist, 400 for a configuration the service refuses, 500 otherwise.
func respondSemanticRouterError(c *gin.Context, err error) {
	var verr *sr.ValidationError
	switch {
	case errors.Is(err, semantic_router.ErrEnterpriseFeature):
		simpleError(c, http.StatusPaymentRequired, "Payment Required", err.Error())
	case errors.Is(err, semantic_router.ErrNotFound):
		simpleError(c, http.StatusNotFound, "Not Found", "Semantic router not found")
	case errors.As(err, &verr), errors.Is(err, semantic_router.ErrInvalid), errors.Is(err, services.ErrEmbedderInvalid),
		errors.Is(err, models.ErrRouteSlugTaken), errors.Is(err, models.ErrUnsafeLogoURL):
		simpleError(c, http.StatusBadRequest, "Bad Request", err.Error())
	default:
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
	}
}
