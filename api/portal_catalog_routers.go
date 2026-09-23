package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// Model Routers in the portal catalog. A router is published in LLM
// catalogues and granted to Apps like an LLM; a client calls it on the
// unified ingress with {"model": "{slug}/{model}"} and the gateway picks the
// LLM. The catalog shows what a developer needs to call it: the model
// strings, and which LLMs a request may end up at (its privacy score is the
// lowest of theirs). Pool patterns, weights and mappings are configuration
// and stay out.

// CatalogRouterLLM is one LLM a router can send a request to.
type CatalogRouterLLM struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
}

func modelRouterCatalogItem(r *models.ModelRouter, privacy map[uint]int) CatalogItem {
	var score *int
	if s, ok := privacy[r.ID]; ok {
		score = intPtr(s)
	}
	short := r.ShortDescription
	if short == "" {
		short = r.Description
	}
	modelStrings := []string{}
	for _, m := range r.AdvertisedModels() {
		modelStrings = append(modelStrings, r.Slug+"/"+m)
	}
	return CatalogItem{Type: CatalogItemModelRouter, ID: uintID(r.ID), Attributes: CatalogItemAttributes{
		Name:                cleanText(r.Name),
		ShortDescription:    cleanText(short),
		LongDescription:     cleanText(r.LongDescription),
		LogoURL:             r.LogoURL,
		Kind:                r.APICompat,
		KindLabel:           "Model Router",
		PrivacyScore:        score,
		Tags:                []string{},
		Catalogs:            []CatalogRef{},
		CreatedAt:           timePtr(r.CreatedAt),
		UpdatedAt:           timePtr(r.UpdatedAt),
		AccessGrantedViaApp: true,
		RouterSlug:          r.Slug,
		RouterModels:        modelStrings,
	}}
}

// getPortalCatalogModelRouter godoc
// @Summary One Model Router from the portal catalog
// @Description The catalog entry for a Model Router the user can see: the model strings to call it with and the LLMs it can route to.
// @Tags common
// @Produce json
// @Param id path int true "Model Router ID"
// @Success 200 {object} CatalogItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /common/catalog/model-routers/{id} [get]
func (a *API) getPortalCatalogModelRouter(c *gin.Context) {
	item, ok := a.findCatalogItem(c, CatalogItemModelRouter, c.Param("id"))
	if !ok {
		return
	}
	id, _ := strconv.ParseUint(item.ID, 10, 64)
	llms, err := a.service.ModelRouterReachableLLMs(uint(id))
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}
	item.Attributes.RouterLLMs = make([]CatalogRouterLLM, 0, len(llms))
	for _, l := range llms {
		item.Attributes.RouterLLMs = append(item.Attributes.RouterLLMs, CatalogRouterLLM{
			ID: uintID(l.ID), Name: cleanText(l.Name), Vendor: string(l.Vendor),
		})
	}
	c.JSON(http.StatusOK, CatalogItemResponse{Data: *item})
}
