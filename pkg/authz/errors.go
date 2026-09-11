package authz

// Error codes carried in the "code" field of an ErrorBody. The UI keys off
// these, never off the HTTP status alone, so it can tell "your role lacks
// this" apart from "this edition lacks this".
const (
	CodeUnauthenticated    = "unauthenticated"
	CodePermissionDenied   = "permission_denied"
	CodeEnterpriseRequired = "enterprise_required"
	CodeSystemRole         = "system_role"
	CodeLastOwner          = "last_owner"
	CodeOwnerUserOnly      = "owner_user_only"
	CodeOwnerRequired      = "owner_required"
)

// Error is one entry in the API error envelope. Title and Detail match the
// shape used by models.ErrorResponse so existing clients keep working.
type Error struct {
	Title      string `json:"title"`
	Detail     string `json:"detail"`
	Code       string `json:"code,omitempty"`
	Permission string `json:"permission,omitempty"`
}

// ErrorBody is the JSON envelope returned on authorization failures.
type ErrorBody struct {
	Errors []Error `json:"errors"`
}

// Denied builds the 403 body for a missing permission. The title stays
// "Forbidden" because existing tests and clients look for that word.
func Denied(p Permission) ErrorBody {
	detail := "you do not have permission to perform this action"
	perm := ""
	if !p.IsSentinel() {
		detail = "missing permission " + string(p)
		perm = string(p)
	} else if p == FullAdmin {
		detail = "this action requires full administrator access"
	}
	return ErrorBody{Errors: []Error{{
		Title:      "Forbidden",
		Detail:     detail,
		Code:       CodePermissionDenied,
		Permission: perm,
	}}}
}

// Unauthenticated builds the 401 body.
func Unauthenticated() ErrorBody {
	return ErrorBody{Errors: []Error{{
		Title:  "Unauthorized",
		Detail: "authentication required",
		Code:   CodeUnauthenticated,
	}}}
}

// EnterpriseRequired builds the 402 body for management endpoints that are
// not available in this edition or licence.
func EnterpriseRequired(detail string) ErrorBody {
	return ErrorBody{Errors: []Error{{
		Title:  "Enterprise Feature",
		Detail: detail,
		Code:   CodeEnterpriseRequired,
	}}}
}

// Body builds an envelope with an arbitrary code.
func Body(title, detail, code string) ErrorBody {
	return ErrorBody{Errors: []Error{{Title: title, Detail: detail, Code: code}}}
}
