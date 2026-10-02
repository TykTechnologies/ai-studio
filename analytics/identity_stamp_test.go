package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/authidentity"
	"github.com/stretchr/testify/assert"
)

// The subject and acting agent an auth plugin named go on the proxy log and
// chat record of the request, whichever record function writes them.
func TestRecordStampsTheAuthIdentity(t *testing.T) {
	withHandler(t, &pairHandler{})
	ctx := authidentity.With(context.Background(), &authidentity.Identity{
		AppID: 1, Method: authidentity.MethodPlugin, Subject: "alice@example.com",
		Claims: map[string]string{authidentity.ClaimActor: "agent-7"},
	})

	log := &models.ProxyLog{AppID: 1, TimeStamp: time.Now()}
	RecordProxyLog(ctx, log)
	assert.Equal(t, "alice@example.com", log.OnBehalfOf)
	assert.Equal(t, "agent-7", log.ActingAgent)

	log2 := &models.ProxyLog{AppID: 1, TimeStamp: time.Now()}
	rec := &models.LLMChatRecord{AppID: 1}
	RecordExchange(ctx, log2, rec)
	assert.Equal(t, "alice@example.com", log2.OnBehalfOf)
	assert.Equal(t, "alice@example.com", rec.OnBehalfOf)
	assert.Equal(t, "agent-7", rec.ActingAgent)

	rec2 := &models.LLMChatRecord{AppID: 1}
	RecordChatRecord(ctx, rec2)
	assert.Equal(t, "agent-7", rec2.ActingAgent)
}

// Without a plugin-named subject (an app key, no identity at all) nothing is
// stamped, and a value a writer set is kept.
func TestRecordLeavesIdentityAlone(t *testing.T) {
	withHandler(t, &pairHandler{})

	appKey := authidentity.With(context.Background(), &authidentity.Identity{AppID: 1, Method: authidentity.MethodAppKey})
	log := &models.ProxyLog{AppID: 1}
	RecordProxyLog(appKey, log)
	assert.Empty(t, log.OnBehalfOf)
	assert.Empty(t, log.ActingAgent)

	RecordProxyLog(context.Background(), log)
	assert.Empty(t, log.OnBehalfOf)

	plugin := authidentity.With(context.Background(), &authidentity.Identity{AppID: 1, Method: authidentity.MethodPlugin, Subject: "alice"})
	preset := &models.ProxyLog{AppID: 1, OnBehalfOf: "set-by-writer"}
	RecordProxyLog(plugin, preset)
	assert.Equal(t, "set-by-writer", preset.OnBehalfOf)
}
