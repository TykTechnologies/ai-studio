package rbac

import "strconv"

// WorkspaceScopeID is the canonical scope_id of a binding scoped to the
// Workspace: its id in decimal, no sign, no leading zeros, no spaces.
// role_bindings.scope_id is a string (it is part of the unique index), so
// exactly one spelling may exist per Workspace or "01" and "1" would be
// two bindings that both grant W1 and escape the uniqueness and in-use
// checks (.39).
func WorkspaceScopeID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

// ParseWorkspaceScopeID reads a canonical Workspace scope_id. It reports
// false for anything WorkspaceScopeID would not have produced, and for 0:
// such a binding grants nothing.
func ParseWorkspaceScopeID(s string) (uint, bool) {
	id, err := strconv.ParseUint(s, 10, 32)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != s {
		return 0, false
	}
	return uint(id), true
}
