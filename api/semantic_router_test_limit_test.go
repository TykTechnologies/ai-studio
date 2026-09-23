package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stretchr/testify/assert"
)

func TestWindowLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newWindowLimiter(2, time.Minute)
	l.now = func() time.Time { return now }

	ok, _ := l.allow("1")
	assert.True(t, ok)
	ok, _ = l.allow("1")
	assert.True(t, ok)
	ok, wait := l.allow("1")
	assert.False(t, ok, "the third run in the window is refused")
	assert.Equal(t, time.Minute, wait)
	ok, _ = l.allow("2")
	assert.True(t, ok, "limits are per user")

	now = now.Add(61 * time.Second)
	ok, _ = l.allow("1")
	assert.True(t, ok, "a new window starts")
	assert.Len(t, l.counts, 1, "stale windows are dropped")

	assert.Equal(t, "1", retryAfterSeconds(0))
	assert.Equal(t, "60", retryAfterSeconds(59*time.Second+time.Millisecond))
}

func TestSemanticRouterTest_RateLimited(t *testing.T) {
	api, _, _ := setupTestAPIForCommonTests(t)
	orig := semanticRouterTests
	semanticRouterTests = newWindowLimiter(0, time.Minute)
	t.Cleanup(func() { semanticRouterTests = orig })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("POST", "/semantic-routers/test",
		bytes.NewBufferString(`{"router":{"name":"S","slug":"s"},"messages":[{"role":"user","content":"hi"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	api.testDraftSemanticRouter(c)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}
