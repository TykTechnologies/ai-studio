package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"

	"github.com/TykTechnologies/midsommar/v2/pkg/authz"
	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/gin-gonic/gin"
)

// The push precondition (Workspaces C2, design §5): a push request may carry
// {"only_workspace_id": <id>}. The push is then refused (409 push_stained)
// while any target namespace has a pending change from outside that
// Workspace, and nothing is recorded. Without the body a push behaves as
// before. The Operator decides what to do with a stained push; Studio only
// refuses when asked to.

// CodePushStained is the error code of a refused stained push.
const CodePushStained = "push_stained"

// maxStainedChangesListed caps the foreign changes a push_stained body lists.
const maxStainedChangesListed = 20

// PushPreconditionInput is the optional body of the push routes.
type PushPreconditionInput struct {
	// OnlyWorkspaceID refuses the push while changes from outside this
	// Workspace are pending in a target namespace.
	OnlyWorkspaceID *uint `json:"only_workspace_id,omitempty"`
}

// pushOnlyWorkspace reads the optional push precondition. It returns the
// Workspace (empty when the request has no precondition) and false when it
// has answered: 400 for a malformed body, 422 invalid_workspace for a
// Workspace that does not exist.
func (a *API) pushOnlyWorkspace(c *gin.Context) ([]uint, bool) {
	if c.Request.Body == nil {
		return nil, true
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<16))
	if err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "could not read the request body")
		c.Abort()
		return nil, false
	}
	if len(raw) == 0 {
		return nil, true
	}
	var in PushPreconditionInput
	if err := json.Unmarshal(raw, &in); err != nil {
		simpleError(c, http.StatusBadRequest, "Bad Request", "the body must be {\"only_workspace_id\": <Workspace id>}")
		c.Abort()
		return nil, false
	}
	if in.OnlyWorkspaceID == nil {
		return nil, true
	}
	exists, err := a.service.WorkspaceExists(*in.OnlyWorkspaceID)
	if err != nil {
		simpleError(c, http.StatusInternalServerError, "Internal Server Error", err.Error())
		c.Abort()
		return nil, false
	}
	if !exists {
		invalidWorkspace(c)
		return nil, false
	}
	return []uint{*in.OnlyWorkspaceID}, true
}

// stainedChange is one foreign change listed in a push_stained body.
type stainedChange struct {
	services.PendingChange
	Namespace string `json:"namespace"`
}

// PushStainedBody is the 409 push_stained answer: what is pending from
// outside the Workspace in the stained target namespaces, redacted for the
// caller as the pending-changes preview is (.44).
type PushStainedBody struct {
	authz.ErrorBody
	WorkspaceID uint `json:"workspace_id"`
	// OutsideTotal counts every foreign change; ByWorkspace splits those
	// owned by a Workspace, UnattributedTotal the rest (global types).
	OutsideTotal      int                              `json:"outside_total"`
	ByWorkspace       []services.PendingWorkspaceTotal `json:"by_workspace"`
	UnattributedTotal int                              `json:"unattributed_total"`
	// Changes lists up to 20 foreign changes, newest first.
	Changes []stainedChange `json:"changes"`
}

// sendPushStained answers 409 push_stained when err is a stained-push
// refusal, and reports whether it did.
func (a *API) sendPushStained(c *gin.Context, err error) bool {
	var stained *services.PushStainedError
	if !errors.As(err, &stained) {
		return false
	}
	ws := stained.WorkspaceID
	body := PushStainedBody{
		ErrorBody:   authz.Body("Conflict", "changes from outside the Workspace are pending in a target namespace; the push would carry them", CodePushStained),
		WorkspaceID: ws,
		ByWorkspace: []services.PendingWorkspaceTotal{},
		Changes:     []stainedChange{},
	}
	byWS := map[uint]*services.PendingWorkspaceTotal{}
	for _, pc := range stained.Stained {
		a.redactPendingChanges(c, pc)
		if pc.OutsideTotal != nil {
			body.OutsideTotal += *pc.OutsideTotal
		}
		for _, t := range pc.ByWorkspace {
			if t.WorkspaceID == ws {
				continue
			}
			if cur, ok := byWS[t.WorkspaceID]; ok {
				cur.Total += t.Total
			} else {
				t := t
				byWS[t.WorkspaceID] = &t
			}
		}
		for _, ch := range pc.Changes {
			if services.PendingChangeIsForeign(ch.Type, ch.WorkspaceID, ws) {
				body.Changes = append(body.Changes, stainedChange{PendingChange: ch, Namespace: pc.Namespace})
			}
		}
	}
	attributed := 0
	for _, t := range byWS {
		body.ByWorkspace = append(body.ByWorkspace, *t)
		attributed += t.Total
	}
	sort.Slice(body.ByWorkspace, func(i, j int) bool { return body.ByWorkspace[i].WorkspaceID < body.ByWorkspace[j].WorkspaceID })
	if body.UnattributedTotal = body.OutsideTotal - attributed; body.UnattributedTotal < 0 {
		body.UnattributedTotal = 0
	}
	sort.SliceStable(body.Changes, func(i, j int) bool { return body.Changes[i].At.After(body.Changes[j].At) })
	if len(body.Changes) > maxStainedChangesListed {
		body.Changes = body.Changes[:maxStainedChangesListed]
	}
	c.AbortWithStatusJSON(http.StatusConflict, body)
	return true
}
