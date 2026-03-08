// Package authz provides fine-grained authorization using an embedded OpenFGA server.
//
// It replaces the existing boolean/JOIN-based access control with a relationship-based
// authorization model. The package runs OpenFGA in-process with an in-memory datastore
// and keeps tuples synchronized with the GORM database.
package authz

import (
	"context"
	"fmt"
	"strconv"
)

// Authorizer defines the interface for authorization checks.
type Authorizer interface {
	// Check returns true if user has the given relation on the object.
	Check(ctx context.Context, userID uint, relation string, objectType string, objectID uint) (bool, error)

	// CheckStr is like Check but accepts a string object ID (for composite keys like plugin resources).
	CheckStr(ctx context.Context, userID uint, relation string, objectType string, objectID string) (bool, error)

	// ListObjects returns object IDs of the given type where the user has the given relation.
	ListObjects(ctx context.Context, userID uint, relation string, objectType string) ([]uint, error)

	// WriteTuples writes relationship tuples to the store.
	WriteTuples(ctx context.Context, writes []Tuple) error

	// DeleteTuples removes relationship tuples from the store.
	DeleteTuples(ctx context.Context, deletes []Tuple) error

	// WriteTuplesAndDelete atomically writes and deletes tuples in one call.
	WriteTuplesAndDelete(ctx context.Context, writes []Tuple, deletes []Tuple) error

	// Close shuts down the embedded OpenFGA server.
	Close()
}

// Tuple represents a single OpenFGA relationship tuple.
type Tuple struct {
	User     string // e.g. "user:42" or "group:5#member"
	Relation string // e.g. "member", "viewer", "admin"
	Object   string // e.g. "system:1", "catalogue:3", "app:7"
}

// Helper functions for constructing tuple strings.

// UserStr returns the OpenFGA user string for a user ID.
func UserStr(id uint) string {
	return "user:" + strconv.FormatUint(uint64(id), 10)
}

// GroupStr returns the OpenFGA object string for a group ID.
func GroupStr(id uint) string {
	return "group:" + strconv.FormatUint(uint64(id), 10)
}

// GroupMemberStr returns the OpenFGA user string for group membership (userset).
func GroupMemberStr(id uint) string {
	return "group:" + strconv.FormatUint(uint64(id), 10) + "#member"
}

// ObjectStr returns the OpenFGA object string for a typed object.
func ObjectStr(objectType string, id uint) string {
	return objectType + ":" + strconv.FormatUint(uint64(id), 10)
}

// ParseObjectID extracts the numeric ID from an OpenFGA object string like "llm:42".
func ParseObjectID(object string) (uint, error) {
	for i := len(object) - 1; i >= 0; i-- {
		if object[i] == ':' {
			id, err := strconv.ParseUint(object[i+1:], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid object ID in %q: %w", object, err)
			}
			return uint(id), nil
		}
	}
	return 0, fmt.Errorf("invalid object string: %q", object)
}

// maxTuplesPerWrite is the OpenFGA limit for tuples in a single Write call.
const maxTuplesPerWrite = 100
