package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// Backend is a Redis-backed rate limit storage using sorted sets.
// Each key maps to a sorted set where members are unique IDs and
// scores are Unix timestamps in nanoseconds.
type Backend struct {
	client    goredis.Cmdable
	keyPrefix string
}

// New creates a Redis backend. The keyPrefix namespaces all keys
// to avoid collisions with other Redis usage.
func New(client goredis.Cmdable, keyPrefix string) *Backend {
	return &Backend{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

func (b *Backend) key(k string) string {
	return b.keyPrefix + k
}

func (b *Backend) Record(ctx context.Context, key string, window time.Duration) (int, error) {
	k := b.key(key)
	now := time.Now()
	cutoff := now.Add(-window)

	pipe := b.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, k, "-inf", fmt.Sprintf("%d", cutoff.UnixNano()))
	pipe.ZAdd(ctx, k, goredis.Z{
		Score:  float64(now.UnixNano()),
		Member: uuid.New().String(),
	})
	countCmd := pipe.ZCard(ctx, k)
	pipe.Expire(ctx, k, window+time.Minute)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("redis record pipeline: %w", err)
	}

	count, err := countCmd.Result()
	if err != nil {
		return 0, fmt.Errorf("redis zcard: %w", err)
	}
	return int(count), nil
}

func (b *Backend) Count(ctx context.Context, key string, window time.Duration) (int, error) {
	k := b.key(key)
	now := time.Now()
	cutoff := now.Add(-window)

	count, err := b.client.ZCount(ctx, k, fmt.Sprintf("%d", cutoff.UnixNano()), "+inf").Result()
	if err != nil {
		return 0, fmt.Errorf("redis zcount: %w", err)
	}
	return int(count), nil
}

func (b *Backend) Reset(ctx context.Context, key string) error {
	if err := b.client.Del(ctx, b.key(key)).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

func (b *Backend) Oldest(ctx context.Context, key string, window time.Duration) (time.Time, error) {
	k := b.key(key)
	cutoff := time.Now().Add(-window)

	members, err := b.client.ZRangeByScoreWithScores(ctx, k, &goredis.ZRangeBy{
		Min:    fmt.Sprintf("%d", cutoff.UnixNano()),
		Max:    "+inf",
		Offset: 0,
		Count:  1,
	}).Result()
	if err != nil {
		return time.Time{}, fmt.Errorf("redis zrangebyscore: %w", err)
	}
	if len(members) == 0 {
		return time.Time{}, nil
	}

	nanos := int64(members[0].Score)
	return time.Unix(0, nanos), nil
}
