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

	allowed, err = noop.CheckStr(ctx, 1, "admin", "system", "1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestNoopAuthorizer_ListObjectsReturnsNil(t *testing.T) {
	ctx := context.Background()
	noop := &NoopAuthorizer{}

	ids, err := noop.ListObjects(ctx, 1, "can_use", "llm")
	require.NoError(t, err)
	assert.Nil(t, ids)

	strs, err := noop.ListObjectsStr(ctx, 1, "can_use", "llm")
	require.NoError(t, err)
	assert.Nil(t, strs)
}

func TestNoopAuthorizer_WritesAreNoOps(t *testing.T) {
	ctx := context.Background()
	noop := &NoopAuthorizer{}

	assert.NoError(t, noop.WriteTuples(ctx, []Tuple{{User: "user:1", Relation: "admin", Object: "system:1"}}))
	assert.NoError(t, noop.DeleteTuples(ctx, []Tuple{{User: "user:1", Relation: "admin", Object: "system:1"}}))
	assert.NoError(t, noop.WriteTuplesAndDelete(ctx, nil, nil))
}
