package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimitResponse struct {
	Allowed  bool
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Err      error
}

type RateLimiter interface {
	IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error)
	Reset(ctx context.Context, key string) error
}

type StrategyConstructor interface {
	Name() string
	NewFromConfig(config map[string]interface{}, redisClient *redis.Client) (RateLimiter, error)
	ConvertConfig(rawConfig interface{}) (map[string]interface{}, error)
}

type RateLimitStrategy string

const (
	TokenBucketStrategy          RateLimitStrategy = "token_bucket"
	SlidingWindowLogStrategy     RateLimitStrategy = "sliding_window_log"
	SlidingWindowCounterStrategy RateLimitStrategy = "sliding_window_counter"
)
