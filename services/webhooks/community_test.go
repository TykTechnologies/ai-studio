//go:build !enterprise
// +build !enterprise

package webhooks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Without the enterprise factory every operation must refuse with
// ErrEnterpriseFeature and the lifecycle methods must be harmless no-ops.
func TestCommunityService(t *testing.T) {
	require.False(t, IsEnterpriseAvailable())
	svc := NewService(Deps{})
	ctx := context.Background()
	actor := Actor{UserID: 1, Email: "admin@tyk.io"}

	_, err := svc.ListTargets(ctx, TargetFilter{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.GetTarget(ctx, "t1")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, _, err = svc.CreateTarget(ctx, actor, TargetInput{URL: "https://example.com"})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, _, err = svc.UpdateTarget(ctx, actor, "t1", 0, TargetPatch{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, svc.DeleteTarget(ctx, actor, "t1"), ErrEnterpriseFeature)
	_, err = svc.ApproveTarget(ctx, actor, "t1", "")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.RejectTarget(ctx, actor, "t1", "no")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.RevokeTarget(ctx, actor, "t1", "no")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.PauseTarget(ctx, actor, "t1")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.ResumeTarget(ctx, actor, "t1")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.RotateSecret(ctx, actor, "t1")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.SendTest(ctx, actor, "t1", "")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.PreviewTemplate(ctx, PreviewInput{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.Empty(t, svc.ListPresets())
	_, err = svc.ListTopics(ctx)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.ListDeliveries(ctx, DeliveryQuery{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.GetDelivery(ctx, "d1")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.ReplayDelivery(ctx, actor, "d1", false)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	assert.ErrorIs(t, svc.CancelDelivery(ctx, actor, "d1"), ErrEnterpriseFeature)
	_, err = svc.ReplayDeadLetters(ctx, actor, ReplayRequest{})
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.Stats(ctx, "24h")
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, _, err = svc.Export(ctx, DeliveryQuery{}, FormatCSV)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)
	_, err = svc.Cleanup(ctx)
	assert.ErrorIs(t, err, ErrEnterpriseFeature)

	st := svc.Status()
	assert.False(t, st.Available)
	assert.False(t, st.Enabled)
	assert.NotPanics(t, svc.Stop)
}
