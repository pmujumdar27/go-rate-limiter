package ratelimit

import (
	"context"
	"time"
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

type RateLimitStrategy string

const (
	TokenBucketStrategy   RateLimitStrategy = "token_bucket"
	SlidingWindowStrategy RateLimitStrategy = "sliding_window"
)
