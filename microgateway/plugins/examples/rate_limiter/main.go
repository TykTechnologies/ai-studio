// plugins/examples/rate_limiter/main.go
package main

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/microgateway/plugins/sdk"
)

// RateLimiterPlugin implements a simple rate limiter for pre-authentication
type RateLimiterPlugin struct {
	mu              sync.RWMutex
	buckets         map[string]*TokenBucket
	requestsPerMin  int
	burstSize       int
	keyExtractor    string
	cleanupTicker   *time.Ticker
	cleanupStop     chan bool
}

// TokenBucket implements a simple token bucket for rate limiting
type TokenBucket struct {
	mu           sync.Mutex
	tokens       int
	lastRefill   time.Time
	maxTokens    int
	refillRate   int    // tokens per minute
}

func NewTokenBucket(maxTokens, refillRate int) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		lastRefill: time.Now(),
		maxTokens:  maxTokens,
		refillRate: refillRate,
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	
	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(tb.lastRefill)
	tokensToAdd := int(elapsed.Minutes() * float64(tb.refillRate))
	
	if tokensToAdd > 0 {
		tb.tokens += tokensToAdd
		if tb.tokens > tb.maxTokens {
			tb.tokens = tb.maxTokens
		}
		tb.lastRefill = now
	}

	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	
	return false
}

func (tb *TokenBucket) Remaining() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

func (tb *TokenBucket) ResetTime() time.Time {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.lastRefill.Add(time.Minute)
}

func (tb *TokenBucket) RetryAfter() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return int64(60 - time.Since(tb.lastRefill).Seconds())
}

// Initialize implements BasePlugin
func (p *RateLimiterPlugin) Initialize(config map[string]interface{}) error {
	p.buckets = make(map[string]*TokenBucket)
	
	// Parse configuration
	if rpm, ok := config["requests_per_minute"]; ok {
		if rpmInt, err := strconv.Atoi(fmt.Sprintf("%v", rpm)); err == nil {
			p.requestsPerMin = rpmInt
		} else {
			p.requestsPerMin = 60 // default
		}
	} else {
		p.requestsPerMin = 60 // default
	}
	
	if burst, ok := config["burst_size"]; ok {
		if burstInt, err := strconv.Atoi(fmt.Sprintf("%v", burst)); err == nil {
			p.burstSize = burstInt
		} else {
			p.burstSize = p.requestsPerMin // default to same as requests per minute
		}
	} else {
		p.burstSize = p.requestsPerMin // default
	}
	
	if extractor, ok := config["key_extractor"]; ok {
		p.keyExtractor = fmt.Sprintf("%v", extractor)
	} else {
		p.keyExtractor = "ip" // default
	}
	
	// Start cleanup routine
	p.cleanupTicker = time.NewTicker(5 * time.Minute)
	p.cleanupStop = make(chan bool)
	go p.cleanupExpiredBuckets()
	
	return nil
}

// GetHookType implements BasePlugin
func (p *RateLimiterPlugin) GetHookType() sdk.HookType {
	return sdk.HookTypePreAuth
}

// GetName implements BasePlugin
func (p *RateLimiterPlugin) GetName() string {
	return "rate-limiter"
}

// GetVersion implements BasePlugin
func (p *RateLimiterPlugin) GetVersion() string {
	return "1.0.0"
}

// Shutdown implements BasePlugin
func (p *RateLimiterPlugin) Shutdown() error {
	if p.cleanupTicker != nil {
		p.cleanupTicker.Stop()
	}
	if p.cleanupStop != nil {
		close(p.cleanupStop)
	}
	return nil
}

// ProcessRequest implements PreAuthPlugin
func (p *RateLimiterPlugin) ProcessRequest(ctx context.Context, req *sdk.PluginRequest, pluginCtx *sdk.PluginContext) (*sdk.PluginResponse, error) {
	// Extract rate limit key based on configuration
	key := p.extractKey(req, pluginCtx)
	
	// Get or create token bucket
	bucket := p.getBucket(key)
	
	// Check rate limit
	if !bucket.Allow() {
		return &sdk.PluginResponse{
			Modified:   true,
			StatusCode: 429,
			Headers: map[string]string{
				"Content-Type":              "application/json",
				"X-RateLimit-Limit":         fmt.Sprintf("%d", p.requestsPerMin),
				"X-RateLimit-Remaining":     "0",
				"X-RateLimit-Reset":         fmt.Sprintf("%d", bucket.ResetTime().Unix()),
				"Retry-After":               fmt.Sprintf("%d", bucket.RetryAfter()),
			},
			Body:         []byte(`{"error": "rate limit exceeded"}`),
			Block:        true,
			ErrorMessage: "",
		}, nil
	}
	
	// Add rate limit headers to response but allow request to continue
	return &sdk.PluginResponse{
		Modified: true,
		Headers: map[string]string{
			"X-RateLimit-Limit":     fmt.Sprintf("%d", p.requestsPerMin),
			"X-RateLimit-Remaining": fmt.Sprintf("%d", bucket.Remaining()),
		},
		Block: false,
	}, nil
}

func (p *RateLimiterPlugin) extractKey(req *sdk.PluginRequest, ctx *sdk.PluginContext) string {
	switch p.keyExtractor {
	case "app":
		return fmt.Sprintf("app:%d:%d", ctx.LLMID, ctx.AppID)
	case "user":
		return fmt.Sprintf("user:%d:%d", ctx.LLMID, ctx.UserID)
	default: // "ip"
		return fmt.Sprintf("ip:%d:%s", ctx.LLMID, req.RemoteAddr)
	}
}

func (p *RateLimiterPlugin) getBucket(key string) *TokenBucket {
	p.mu.RLock()
	bucket, exists := p.buckets[key]
	p.mu.RUnlock()
	
	if exists {
		return bucket
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Double-check after acquiring write lock
	if bucket, exists := p.buckets[key]; exists {
		return bucket
	}
	
	// Create new bucket
	bucket = NewTokenBucket(p.burstSize, p.requestsPerMin)
	p.buckets[key] = bucket
	return bucket
}

func (p *RateLimiterPlugin) cleanupExpiredBuckets() {
	for {
		select {
		case <-p.cleanupTicker.C:
			p.mu.Lock()
			now := time.Now()
			for key, bucket := range p.buckets {
				// Remove buckets that haven't been accessed in 10 minutes
				if now.Sub(bucket.lastRefill) > 10*time.Minute {
					delete(p.buckets, key)
				}
			}
			p.mu.Unlock()
		case <-p.cleanupStop:
			return
		}
	}
}

func main() {
	plugin := &RateLimiterPlugin{}
	sdk.ServePlugin(plugin)
}