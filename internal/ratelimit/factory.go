package ratelimit

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewRateLimiter(strategy RateLimitStrategy, redisClient *redis.Client, keyPrefix string, config map[string]interface{}) (RateLimiter, error) {
	switch strategy {
	case TokenBucketStrategy:
		bucketSize, err := getInt64Config(config, "bucket_size")
		if err != nil {
			return nil, fmt.Errorf("token bucket strategy: %w", err)
		}
		refillRate, err := getInt64Config(config, "refill_rate_per_second")
		if err != nil {
			return nil, fmt.Errorf("token bucket strategy: %w", err)
		}
		tokenBucketConfig := TokenBucketConfig{
			BucketSize:          bucketSize,
			RefillRatePerSecond: refillRate,
			KeyPrefix:           keyPrefix,
		}
		return NewTokenBucketRateLimiter(tokenBucketConfig, redisClient)

	case SlidingWindowStrategy:
		windowSize, err := getDurationConfig(config, "window_size")
		if err != nil {
			return nil, fmt.Errorf("sliding window strategy: %w", err)
		}
		bucketSize, err := getInt64Config(config, "bucket_size")
		if err != nil {
			return nil, fmt.Errorf("sliding window strategy: %w", err)
		}
		slidingWindowLogConfig := SlidingWindowLogConfig{
			WindowSize: windowSize,
			BucketSize: bucketSize,
			KeyPrefix:  keyPrefix,
		}
		return NewSlidingWindowLogRateLimiter(slidingWindowLogConfig, redisClient)

	case SlidingWindowCounterStrategy:
		windowSize, err := getDurationConfig(config, "window_size")
		if err != nil {
			return nil, fmt.Errorf("sliding window counter strategy: %w", err)
		}
		bucketSize, err := getInt64Config(config, "bucket_size")
		if err != nil {
			return nil, fmt.Errorf("sliding window counter strategy: %w", err)
		}
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

func getInt64Config(config map[string]interface{}, key string) (int64, error) {
	value, exists := config[key]
	if !exists {
		return 0, fmt.Errorf("required config key '%s' not found", key)
	}

	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("config key '%s' must be a number, got %T", key, value)
	}
}

func getDurationConfig(config map[string]interface{}, key string) (time.Duration, error) {
	value, exists := config[key]
	if !exists {
		return 0, fmt.Errorf("required config key '%s' not found", key)
	}

	if duration, ok := value.(time.Duration); ok {
		return duration, nil
	}

	return 0, fmt.Errorf("config key '%s' must be a time.Duration, got %T", key, value)
}
