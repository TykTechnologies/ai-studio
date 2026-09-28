package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Router returns the router instance
func (a *API) Router() *gin.Engine {
	return a.router
}

// Handler returns the API and UI for serving under the configured base path.
// Routes are registered at the root; the handler strips the base path.
func (a *API) Handler() http.Handler {
	return withBasePath(a.basePath, a.router)
}
