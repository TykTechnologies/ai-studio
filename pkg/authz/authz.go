// Package authz provides fine-grained relationship-based authorization.
//
// It defines an abstract Authorizer interface for checking permissions using
// a subject/relation/resource model. The interface is backend-agnostic — the
// current implementation uses an embedded OpenFGA server, but any authorization
// engine that can evaluate relationship graphs could be substituted.
//
// The feature is controlled by the OPENFGA_ENABLED environment variable.
// When disabled, a NoopAuthorizer is used that always permits access,
// deferring all authorization decisions to the legacy auth system.
package authz

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Authorizer defines the interface for authorization checks.
// The interface uses domain-neutral terminology:
//   - Subject: the actor (typically "user:<id>")
//   - Relation: the permission or role (e.g. "viewer", "editor", "admin")
//   - Resource: the object being accessed (e.g. "llm:<id>", "app:<id>")
type Authorizer interface {
	// Enabled returns true if the fine-grained authorization system is active.
	Enabled() bool

	// Check returns true if the subject (user) has the given relation on the resource.
	Check(ctx context.Context, userID uint, relation string, resourceType string, resourceID uint) (bool, error)

	// CheckByName is like Check but accepts a string resource ID (for composite keys).
	CheckByName(ctx context.Context, userID uint, relation string, resourceType string, resourceID string) (bool, error)

	// ListResources returns numeric resource IDs of the given type where the user
	// has the given relation. Returns an error if any resource has a non-numeric ID;
	// use ListResourcesByName for those types.
	ListResources(ctx context.Context, userID uint, relation string, resourceType string) ([]uint, error)

	// ListResourcesByName returns raw resource identifiers (e.g. "llm:5",
	// "plugin_resource:3_srv-1") where the user has the given relation.
	ListResourcesByName(ctx context.Context, userID uint, relation string, resourceType string) ([]string, error)

	// Grant writes relationship grants to the authorization store.
	Grant(ctx context.Context, grants []Relationship) error

	// Revoke removes relationship grants from the authorization store.
	Revoke(ctx context.Context, revocations []Relationship) error

	// GrantAndRevoke atomically writes and removes relationships in one call.
	GrantAndRevoke(ctx context.Context, grants []Relationship, revocations []Relationship) error

	// Close shuts down the authorization backend.
	Close()
}

// Relationship represents a single authorization relationship:
// "Subject has Relation on Resource".
type Relationship struct {
	Subject  string // The actor, e.g. "user:42" or "group:5#member"
	Relation string // The permission, e.g. "member", "viewer", "admin"
	Resource string // The target, e.g. "system:1", "catalogue:3", "app:7"
}

// --- Helper functions for constructing subject/resource identifiers ---

// SubjectUser returns the subject identifier for a user.
func SubjectUser(id uint) string {
	return "user:" + strconv.FormatUint(uint64(id), 10)
}

// SubjectGroup returns the resource identifier for a group.
func SubjectGroup(id uint) string {
	return "group:" + strconv.FormatUint(uint64(id), 10)
}

// SubjectGroupMembers returns a subject identifier representing all members of a group.
func SubjectGroupMembers(id uint) string {
	return "group:" + strconv.FormatUint(uint64(id), 10) + "#member"
}

// ResourceID returns the resource identifier for a typed resource with a numeric ID.
func ResourceID(resourceType string, id uint) string {
	return resourceType + ":" + strconv.FormatUint(uint64(id), 10)
}

// ResourceByName returns the resource identifier for a typed resource with a string ID.
// The id is validated to ensure it does not contain colons (which would break parsing).
func ResourceByName(resourceType string, id string) (string, error) {
	if err := validateID(id); err != nil {
		return "", fmt.Errorf("invalid resource ID for type %q: %w", resourceType, err)
	}
	return resourceType + ":" + id, nil
}

// ParseResourceNumericID extracts the numeric ID from a resource string like "llm:42".
// Returns an error for non-numeric IDs — use ParseResourceID for composite IDs.
func ParseResourceNumericID(resource string) (uint, error) {
	raw, err := ParseResourceID(resource)
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("non-numeric ID in resource %q: %w", resource, err)
	}
	return uint(id), nil
}

// ParseResourceID extracts the ID portion after the first colon from a resource
// string. For "llm:42" returns "42". For "plugin_resource:3_srv-1" returns "3_srv-1".
// Uses the first colon as delimiter (not the last) to prevent injection via colons in IDs.
func ParseResourceID(resource string) (string, error) {
	idx := strings.IndexByte(resource, ':')
	if idx < 0 || idx == len(resource)-1 {
		return "", fmt.Errorf("invalid resource identifier: %q", resource)
	}
	return resource[idx+1:], nil
}

// validateID checks that an ID string does not contain characters that would
// break the "type:id" format. Colons are forbidden to prevent injection attacks
// where a malicious ID like "evil:123" would be parsed as type "evil" with ID "123".
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("empty ID")
	}
	if strings.ContainsRune(id, ':') {
		return fmt.Errorf("ID %q contains forbidden colon character", id)
	}
	return nil
}

// maxBatchSize is the backend limit for relationships in a single write call.
const maxBatchSize = 100
