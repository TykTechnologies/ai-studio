package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// allowInternalHostUpdate resolves an asset's allow_internal_host (the
// per-asset "Allow internal host" switch, epic w4e) from a request: an
// omitted value (nil) keeps stored; a change needs an administrator
// (IsAdmin), anyone else is answered 403. On create pass stored false. It
// returns the value to save and whether the handler may go on; when it
// returns false the response has been written.
//
// The switch lets the asset's own hosts be internal addresses, so it is
// never settable by a non-admin, a submission or a plugin (object hooks
// and the plugin service API keep the stored value).
func allowInternalHostUpdate(c *gin.Context, stored bool, requested *bool) (bool, bool) {
	if requested == nil || *requested == stored {
		return stored, true
	}
	if user, ok := currentUser(c); !ok || !user.IsAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{{Title: "Forbidden", Detail: "Only an administrator may change allow_internal_host"}}})
		return stored, false
	}
	return *requested, true
}
