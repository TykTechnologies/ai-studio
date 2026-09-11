// Package authz defines the permission vocabulary used by role-based access
// control. It is deliberately a leaf package: it knows nothing about users,
// roles, or storage. The catalogue of resources lives here so both editions
// share one source of truth, routes can be annotated with typed permissions,
// and the UI can render a permission matrix straight from the API.
//
// A permission is "<resource>:<action>" where the resource is the plural
// kebab-case route collection segment ("llms", "data-catalogues") and the
// action is one of read, write, delete, execute. write, delete and execute
// each imply read.
//
// Community Edition never evaluates these beyond "is the user an admin";
// Enterprise Edition resolves a user's effective set from role bindings.
package authz

import (
	"fmt"
	"strings"
)

// Action is one of the four verbs a permission can carry.
type Action string

const (
	// ActionRead covers list, get, search, status and history reads, and
	// downloading an already-produced artefact.
	ActionRead Action = "read"
	// ActionWrite covers create and update, including linking sub-resources,
	// activate/deactivate, approve/reject and rollback.
	ActionWrite Action = "write"
	// ActionDelete covers removing a resource or a sub-resource link.
	ActionDelete Action = "delete"
	// ActionExecute covers side-effecting operations that do not persist
	// configuration: test, call, reload, sync, re-process.
	ActionExecute Action = "execute"
)

// Actions lists every action in display order.
var Actions = []Action{ActionRead, ActionWrite, ActionDelete, ActionExecute}

// ActionLabels are the human labels the UI shows as matrix columns.
var ActionLabels = map[Action]string{
	ActionRead:    "Read",
	ActionWrite:   "Write",
	ActionDelete:  "Delete",
	ActionExecute: "Execute",
}

// Valid reports whether a is one of the four known actions.
func (a Action) Valid() bool {
	_, ok := ActionLabels[a]
	return ok
}

// Permission is a "<resource>:<action>" string, or one of the two sentinels.
type Permission string

const (
	// FullAdmin is the wildcard held only by the Owner and Administrator
	// system roles. It satisfies every permission check.
	FullAdmin Permission = "*"

	// AnyAdmin is a route annotation sentinel, never stored in a role. A
	// route annotated with it is open to any user who holds at least one
	// permission (i.e. anyone allowed onto the admin surface at all).
	AnyAdmin Permission = "_any"
)

// P builds a permission from a resource key and action.
func P(resource string, a Action) Permission {
	return Permission(resource + ":" + string(a))
}

// Read, Write, Delete and Execute are shorthand constructors used at route
// registration so annotations stay short and greppable.
func Read(resource string) Permission    { return P(resource, ActionRead) }
func Write(resource string) Permission   { return P(resource, ActionWrite) }
func Delete(resource string) Permission  { return P(resource, ActionDelete) }
func Execute(resource string) Permission { return P(resource, ActionExecute) }

// Parse validates a raw permission string against the catalogue. FullAdmin
// is accepted; AnyAdmin is not (it is a route sentinel, not a grant).
func Parse(s string) (Permission, error) {
	p := Permission(strings.TrimSpace(s))
	if p == FullAdmin {
		return p, nil
	}
	if p == AnyAdmin || !p.Valid() {
		return "", fmt.Errorf("unknown permission %q", s)
	}
	return p, nil
}

// String implements fmt.Stringer.
func (p Permission) String() string { return string(p) }

// IsSentinel reports whether p is FullAdmin or AnyAdmin.
func (p Permission) IsSentinel() bool {
	return p == FullAdmin || p == AnyAdmin
}

// Resource returns the resource key, or "" for sentinels.
func (p Permission) Resource() string {
	if p.IsSentinel() {
		return ""
	}
	i := strings.LastIndexByte(string(p), ':')
	if i < 0 {
		return ""
	}
	return string(p[:i])
}

// Action returns the action, or "" for sentinels.
func (p Permission) Action() Action {
	if p.IsSentinel() {
		return ""
	}
	i := strings.LastIndexByte(string(p), ':')
	if i < 0 {
		return ""
	}
	return Action(p[i+1:])
}

// Valid reports whether p is a sentinel or a concrete permission present in
// the catalogue (the resource exists and offers the action).
func (p Permission) Valid() bool {
	if p.IsSentinel() {
		return true
	}
	_, ok := Lookup(p)
	return ok
}
