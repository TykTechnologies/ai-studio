package services

import (
	"context"
	"testing"
	"time"
)

// A gateway running without a plugin manager (no pulse plugin configured) has
// nowhere to forward a tool call. It must drop it quietly rather than panic on
// the request path.
func TestRecordToolCall_NoPluginManager(t *testing.T) {
	handler := NewMicrogatewaAnalyticsHandler(nil, nil, nil, nil)

	handler.RecordToolCall(context.Background(), "getExchangeRate", time.Now(), 120, 4)
}
