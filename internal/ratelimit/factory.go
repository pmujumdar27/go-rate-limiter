package ratelimit

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewRateLimiter(strategy RateLimitStrategy, redisClient *redis.Client, keyPrefix string, config map[string]interface{}) (RateLimiter, error) {
	switch strategy {
	case TokenBucketStrategy:
		bucketSize := config["bucket_size"].(int64)
		refillRate := config["refill_rate_per_second"].(int64)
		tokenBucketConfig := TokenBucketConfig{
			BucketSize:          bucketSize,
			RefillRatePerSecond: refillRate,
			KeyPrefix:           keyPrefix,
		}
		return NewTokenBucketRateLimiter(tokenBucketConfig, redisClient)

	case SlidingWindowStrategy:
		windowSize := config["window_size"].(time.Duration)
		bucketSize := config["bucket_size"].(int64)
		slidingWindowLogConfig := SlidingWindowLogConfig{
			WindowSize: windowSize,
			BucketSize: bucketSize,
			KeyPrefix:  keyPrefix,
		}
		return NewSlidingWindowLogRateLimiter(slidingWindowLogConfig, redisClient)

	case SlidingWindowCounterStrategy:
		windowSize := config["window_size"].(time.Duration)
		bucketSize := config["bucket_size"].(int64)
		slidingWindowCounterConfig := SlidingWindowCounterConfig{
			WindowSize: windowSize,
			BucketSize: bucketSize,
			KeyPrefix:  keyPrefix,
		}
		return NewSlidingWindowCounterRateLimiter(slidingWindowCounterConfig, redisClient)

	default:
		return nil, fmt.Errorf("unsupported rate limiter strategy: %s", strategy)
	}
}
