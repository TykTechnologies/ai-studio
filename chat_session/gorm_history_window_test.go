package chat_session

import (
	"context"
	"fmt"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/driver/sqlite"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/langchaingo/llms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session longer than the history limit sends its newest messages, the
// question being asked among them, with the system prompt kept in front.
func TestHistoryWindowIsTheNewestMessages(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.InitModels(db))
	ctx := context.Background()

	h := NewGormChatMessageHistory(db, "long-session", 1, 1, "system prompt", WithLimit(10))
	for i := 0; i < 25; i++ {
		require.NoError(t, h.AddUserMessage(ctx, fmt.Sprintf("msg %d", i)))
	}

	msgs, err := h.Messages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 11)
	assert.Equal(t, llms.ChatMessageTypeSystem, msgs[0].Role)
	assert.Equal(t, "msg 15", msgs[1].Parts[0].(llms.TextContent).Text, "oldest message of the window")
	assert.Equal(t, "msg 24", msgs[10].Parts[0].(llms.TextContent).Text, "the newest message is sent")

	// The default window (100) on a session of 120 messages ends with the
	// newest one.
	def := NewGormChatMessageHistory(db, "default-limit", 1, 1, "system prompt")
	for i := 0; i < 120; i++ {
		require.NoError(t, def.AddUserMessage(ctx, fmt.Sprintf("msg %d", i)))
	}
	msgs, err = def.Messages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 101)
	assert.Equal(t, llms.ChatMessageTypeSystem, msgs[0].Role)
	assert.Equal(t, "msg 20", msgs[1].Parts[0].(llms.TextContent).Text)
	assert.Equal(t, "msg 119", msgs[100].Parts[0].(llms.TextContent).Text)

	// A short session is returned whole, once.
	short := NewGormChatMessageHistory(db, "short-session", 1, 1, "system prompt", WithLimit(10))
	require.NoError(t, short.AddUserMessage(ctx, "only"))
	msgs, err = short.Messages(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, llms.ChatMessageTypeSystem, msgs[0].Role)
	assert.Equal(t, "only", msgs[1].Parts[0].(llms.TextContent).Text)
}
