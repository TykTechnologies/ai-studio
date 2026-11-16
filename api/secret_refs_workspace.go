package api

import (
	"net/http"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/gin-gonic/gin"
)

// Secret references and Workspaces (midsommar-clean-oztk.38). A $SECRET/
// reference is resolved on the hub and its value sent to the endpoint the
// referencing object names, so adding one is reading the secret: the
// caller's secrets:read must cover the secret's Workspace, and the secret
// must be in the same Workspace as the object (no Default fallback). An
// $ENV/ reference has no owner and needs secrets:read globally. An unknown
// secret name answers the same 403 as one the caller may not use.

// requireSecretRefsIn refuses (403) a write of an object in workspaceID (a
// stored workspace_id; 0 is the Default) that adds or changes references
// (secrets.NewReferences(old, updated)) the caller may not use there. In
// TestMode and for legacy callers the global check applies as before.
func (a *API) requireSecretRefsIn(c *gin.Context, workspaceID uint, old, updated map[string]string) bool {
	keys := secrets.NewReferences(old, updated)
	if len(keys) == 0 {
		return true
	}
	if a.workspaceCheckMode(c) != workspaceCheck {
		return requireSecretReadForKeys(c, keys)
	}
	target, ok := a.effectiveWorkspaceID(c, workspaceID)
	if !ok {
		return false
	}
	deny := func() bool {
		c.AbortWithStatusJSON(http.StatusForbidden, authz.Denied(authz.Read("secrets")))
		return false
	}
	// One query for every referenced name (secrets.SecretWorkspacesByVarNames
	// resolves as a reference does: unknown and ambiguous names are absent).
	var names []string
	for _, k := range keys {
		if v := updated[k]; strings.HasPrefix(v, secrets.SecretRefPrefix) {
			names = append(names, strings.TrimPrefix(v, secrets.SecretRefPrefix))
		}
	}
	held, err := secrets.SecretWorkspacesByVarNames(a.service.DB, names)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	for _, k := range keys {
		v := updated[k]
		if !strings.HasPrefix(v, secrets.SecretRefPrefix) {
			// $ENV/: no owning Workspace.
			if !authz.Can(c, authz.Read("secrets")) {
				return deny()
			}
			continue
		}
		ws, found := held[strings.TrimPrefix(v, secrets.SecretRefPrefix)]
		if !found {
			return deny()
		}
		eff, ok := a.effectiveWorkspaceID(c, ws)
		if !ok {
			return false
		}
		if eff != target || !authz.CanIn(c, eff, authz.Read("secrets")) {
			return deny()
		}
	}
	return true
}

// requireSecretRefsStayIn refuses (422 invalid_workspace) moving an object
// whose credential fields reference a secret outside the destination
// Workspace: the references would point across Workspaces after the move.
func (a *API) requireSecretRefsStayIn(c *gin.Context, destination uint, fields map[string]string) bool {
	if a.workspaceCheckMode(c) != workspaceCheck {
		return true
	}
	var names []string
	for _, v := range fields {
		if strings.HasPrefix(v, secrets.SecretRefPrefix) {
			names = append(names, strings.TrimPrefix(v, secrets.SecretRefPrefix))
		}
	}
	if len(names) == 0 {
		return true
	}
	held, err := secrets.SecretWorkspacesByVarNames(a.service.DB, names)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return false
	}
	for _, name := range names {
		ws, found := held[name]
		eff, ok := a.effectiveWorkspaceID(c, ws)
		if !ok {
			return false
		}
		if !found || eff != destination {
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, authz.Body("Invalid Workspace",
				"the object references a secret in another Workspace; change the reference before moving it", authz.CodeInvalidWorkspace))
			return false
		}
	}
	return true
}
