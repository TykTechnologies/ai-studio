package api

import "github.com/gin-gonic/gin"

// Router returns the router instance
func (a *API) Router() *gin.Engine {
	return a.router
}
