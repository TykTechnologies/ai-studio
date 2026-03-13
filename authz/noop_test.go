package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoopAuthorizer_Enabled(t *testing.T) {
	noop := &NoopAuthorizer{}
	assert.False(t, noop.Enabled())
}

func TestNoopAuthorizer_AlwaysAllows(t *testing.T) {
	ctx := context.Background()
	noop := &NoopAuthorizer{}

	allowed, err := noop.Check(ctx, 1, "admin", "system", 1)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = noop.CheckByName(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestNoopAuthorizer_ListResourcesReturnsNil(t *testing.T) {
	ctx := context.Background()
	noop := &NoopAuthorizer{}

	ids, err := noop.ListResources(ctx, 1, "can_use", "llm")
	require.NoError(t, err)
	assert.Nil(t, ids)

	strs, err := noop.ListResourcesByName(ctx, 1, "can_use", "llm")
	require.NoError(t, err)
	assert.Nil(t, strs)
}

func TestNoopAuthorizer_WritesAreNoOps(t *testing.T) {
	ctx := context.Background()
	noop := &NoopAuthorizer{}

	assert.NoError(t, noop.Grant(ctx, []Relationship{{Subject: "user:1", Relation: "admin", Resource: "system:1"}}))
	assert.NoError(t, noop.Revoke(ctx, []Relationship{{Subject: "user:1", Relation: "admin", Resource: "system:1"}}))
	assert.NoError(t, noop.GrantAndRevoke(ctx, nil, nil))
}
