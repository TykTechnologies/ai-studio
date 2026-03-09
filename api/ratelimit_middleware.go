package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/ratelimit"
	"github.com/TykTechnologies/midsommar/v2/ratelimit/memory"
	rlredis "github.com/TykTechnologies/midsommar/v2/ratelimit/redis"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

type rateLimitEntry struct {
	ipLimiter       *ratelimit.Limiter
	compoundLimiter *ratelimit.Limiter
	fieldLimiter    *ratelimit.Limiter
	fieldName       string
}

func newBackend(ctx context.Context, cfg config.RateLimitConfig) ratelimit.Backend {
	if cfg.Backend == "redis" && cfg.Redis.URL != "" {
		opts, err := goredis.ParseURL(cfg.Redis.URL)
		if err != nil {
			logger.ErrorErr("invalid rate limit Redis URL, falling back to memory", err)
			return memory.New(ctx, time.Minute)
		}
		client := goredis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			logger.ErrorErr("rate limit Redis unreachable, falling back to memory", err)
			return memory.New(ctx, time.Minute)
		}
		logger.Info("rate limiter using Redis backend")
		return rlredis.New(client, cfg.Redis.KeyPrefix)
	}
	logger.Info("rate limiter using in-memory backend")
	return memory.New(ctx, time.Minute)
}

func setupRateLimiters(ctx context.Context, cfg config.RateLimitConfig) map[string]*rateLimitEntry {
	backend := newBackend(ctx, cfg)
	r := cfg.Rules

	return map[string]*rateLimitEntry{
		"login": {
			ipLimiter:       ratelimit.NewLimiter(backend, r.LoginIP.Limit, r.LoginIP.Window),
			compoundLimiter: ratelimit.NewLimiter(backend, r.LoginAccount.Limit, r.LoginAccount.Window),
			fieldName:       "email",
		},
		"register": {
			ipLimiter: ratelimit.NewLimiter(backend, r.Register.Limit, r.Register.Window),
		},
		"forgot-password": {
			fieldLimiter: ratelimit.NewLimiter(backend, r.ForgotPassword.Limit, r.ForgotPassword.Window),
			fieldName:    "email",
		},
		"resend-verification": {
			fieldLimiter: ratelimit.NewLimiter(backend, r.ResendVerify.Limit, r.ResendVerify.Window),
			fieldName:    "email",
		},
		"oauth-token": {
			ipLimiter: ratelimit.NewLimiter(backend, r.OAuthToken.Limit, r.OAuthToken.Window),
		},
	}
}

func rateLimitHandler(entry *rateLimitEntry) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ip := c.ClientIP()

		var fieldValue string
		if entry.fieldName != "" && (entry.compoundLimiter != nil || entry.fieldLimiter != nil) {
			fieldValue = extractField(c, entry.fieldName)
		}

		if entry.ipLimiter != nil {
			if r, err := entry.ipLimiter.Allow(ctx, ip); err == nil && !r.Allowed {
				rejectWithRetry(c, r.RetryAfter)
				return
			}
		}

		if entry.compoundLimiter != nil && fieldValue != "" {
			key := ip + ":" + fieldValue
			if r, err := entry.compoundLimiter.Allow(ctx, key); err == nil && !r.Allowed {
				rejectWithRetry(c, r.RetryAfter)
				return
			}
		}

		if entry.fieldLimiter != nil && fieldValue != "" {
			if r, err := entry.fieldLimiter.Allow(ctx, fieldValue); err == nil && !r.Allowed {
				rejectWithRetry(c, r.RetryAfter)
				return
			}
		}

		c.Next()
	}
}

func extractField(c *gin.Context, field string) string {
	if c.Request.Body == nil {
		return ""
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var parsed map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return ""
	}
	if v, ok := parsed[field]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func rejectWithRetry(c *gin.Context, retryAfter time.Duration) {
	seconds := int(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", fmt.Sprintf("%d", seconds))
	c.JSON(http.StatusTooManyRequests, ErrorResponse{
		Errors: []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}{
			{
				Title:  "Too Many Requests",
				Detail: fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", seconds),
			},
		},
	})
	c.Abort()
}
