package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pmujumdar27/go-rate-limiter/internal/config"
	"github.com/pmujumdar27/go-rate-limiter/internal/metrics"
	"github.com/redis/go-redis/v9"
)

type SlidingWindowLogConfig struct {
	WindowSize       time.Duration
	BucketSize       int64
	KeyPrefix        string
	TTLBufferSeconds int
}

type SlidingWindowLogRateLimiter struct {
	windowSizeSeconds int64
	redisClient       *redis.Client
	keyPrefix         string
	bucketSize        int64
	ttlBuffer         int64
}

func NewSlidingWindowLogRateLimiter(config SlidingWindowLogConfig, redisClient *redis.Client) (*SlidingWindowLogRateLimiter, error) {
	if config.WindowSize <= 0 || config.BucketSize <= 0 || redisClient == nil {
		return nil, errors.New("invalid configuration")
	}

	ttlBufferSeconds := config.TTLBufferSeconds
	if ttlBufferSeconds <= 0 {
		ttlBufferSeconds = DefaultTTLBufferSeconds
	}

	return &SlidingWindowLogRateLimiter{
		windowSizeSeconds: int64(config.WindowSize.Seconds()),
		redisClient:       redisClient,
		keyPrefix:         config.KeyPrefix,
		bucketSize:        config.BucketSize,
		ttlBuffer:         int64(ttlBufferSeconds),
	}, nil
}

func (swl *SlidingWindowLogRateLimiter) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	start := time.Now()
	defer func() {
		metrics.RateLimitDuration.WithLabelValues("sliding_window_log").Observe(time.Since(start).Seconds())
	}()

	redisKey := fmt.Sprintf("%s:%s", swl.keyPrefix, key)

	currentTimestampNanos := timestamp.UnixNano()
	windowStartNanos := currentTimestampNanos - (swl.windowSizeSeconds * NanosecondsPerSecond)

	script := `
		local key = KEYS[1]
		local window_start_nanos = tonumber(ARGV[1])
		local current_timestamp_nanos = tonumber(ARGV[2])
		local bucket_size = tonumber(ARGV[3])
		local window_size_seconds = tonumber(ARGV[4])
		local ttl_buffer_seconds = tonumber(ARGV[5])
		
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start_nanos)
		
		local current_count = redis.call('ZCARD', key)
		
		if current_count >= bucket_size then
			local timestamps = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
			local oldest_timestamp_nanos = 0
			local reset_time_seconds = 0
			
			if #timestamps > 0 then
				oldest_timestamp_nanos = tonumber(timestamps[2])
				reset_time_seconds = (oldest_timestamp_nanos + (window_size_seconds * 1000000000)) / 1000000000 -- NanosecondsPerSecond
			end
			
			return {0, current_count, reset_time_seconds}
		end
		
		local member = current_timestamp_nanos .. ':' .. math.random()
		redis.call('ZADD', key, current_timestamp_nanos, member)
		
		local ttl_seconds = window_size_seconds + ttl_buffer_seconds
		redis.call('EXPIRE', key, ttl_seconds)
		
		local remaining = bucket_size - current_count - 1
		
		return {1, current_count + 1, 0, remaining}
	`

	redisStart := time.Now()
	result, err := swl.redisClient.Eval(ctx, script, []string{redisKey},
		windowStartNanos, currentTimestampNanos, swl.bucketSize, swl.windowSizeSeconds, swl.ttlBuffer).Result()
	metrics.RedisOperationDuration.WithLabelValues("eval").Observe(time.Since(redisStart).Seconds())

	if err != nil {
		metrics.RedisOperations.WithLabelValues("eval", "error").Inc()
		return RateLimitResponse{
			Err: err,
		}, err
	}
	metrics.RedisOperations.WithLabelValues("eval", "success").Inc()

	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) < 3 {
		err = errors.New("invalid redis response from sliding window log script")
		return RateLimitResponse{Err: err}, err
	}

	allowed, err := getInt64FromResult(resultArray[0])
	if err != nil {
		err = fmt.Errorf("failed to parse allowed flag: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	currentCount, err := getInt64FromResult(resultArray[1])
	if err != nil {
		err = fmt.Errorf("failed to parse current count: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	resetTimeSeconds, err := getInt64FromResult(resultArray[2])
	if err != nil {
		err = fmt.Errorf("failed to parse reset time: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	metadata := map[string]interface{}{
		"current_count": currentCount,
		"window_size":   swl.windowSizeSeconds,
	}

	resetTime := timestamp.Add(time.Duration(swl.windowSizeSeconds) * time.Second)
	if resetTimeSeconds > 0 {
		resetTime = time.Unix(resetTimeSeconds, 0)
	}

	if allowed == 1 {
		metrics.RateLimitRequests.WithLabelValues("sliding_window_log", "allowed").Inc()
		remainingRequests := int64(0)
		if len(resultArray) > 3 {
			if remaining, err := getInt64FromResult(resultArray[3]); err == nil {
				remainingRequests = remaining
			}
		}

		return RateLimitResponse{
			Allowed:   true,
			Limit:     swl.bucketSize,
			Remaining: remainingRequests,
			ResetTime: resetTime,
			Metadata:  metadata,
		}, nil
	}

	metrics.RateLimitRequests.WithLabelValues("sliding_window_log", "denied").Inc()
	retryAfter := swl.calculateRetryAfter(&resetTime, timestamp)

	return RateLimitResponse{
		Allowed:    false,
		Limit:      swl.bucketSize,
		Remaining:  0,
		ResetTime:  resetTime,
		RetryAfter: &retryAfter,
		Metadata:   metadata,
	}, nil
}

func (swl *SlidingWindowLogRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:%s", swl.keyPrefix, key)

	_, err := swl.redisClient.Del(ctx, redisKey).Result()
	if err != nil {
		return err
	}

	return nil
}

func (swl *SlidingWindowLogRateLimiter) calculateRetryAfter(resetTime *time.Time, currentTime time.Time) time.Duration {
	if resetTime == nil {
		return 0
	}

	duration := resetTime.Sub(currentTime)
	if duration < 0 {
		duration = 0
	}

	return duration
}

type SlidingWindowLogConstructor struct{}

func (c *SlidingWindowLogConstructor) Name() string {
	return "sliding_window_log"
}

func (c *SlidingWindowLogConstructor) NewFromConfig(config map[string]interface{}, redisClient *redis.Client) (RateLimiter, error) {
	windowSize, err := getDurationConfig(config, "window_size")
	if err != nil {
		return nil, fmt.Errorf("sliding window strategy: %w", err)
	}
	bucketSize, err := getInt64Config(config, "bucket_size")
	if err != nil {
		return nil, fmt.Errorf("sliding window strategy: %w", err)
	}
	keyPrefix, err := getStringConfig(config, "key_prefix")
	if err != nil {
		return nil, fmt.Errorf("sliding window strategy: %w", err)
	}
	ttlBuffer, err := getIntConfig(config, "ttl_buffer_seconds")
	if err != nil {
		return nil, fmt.Errorf("sliding window strategy: %w", err)
	}

	slidingWindowLogConfig := SlidingWindowLogConfig{
		WindowSize:       windowSize,
		BucketSize:       bucketSize,
		KeyPrefix:        keyPrefix,
		TTLBufferSeconds: ttlBuffer,
	}
	return NewSlidingWindowLogRateLimiter(slidingWindowLogConfig, redisClient)
}

func (c *SlidingWindowLogConstructor) ConvertConfig(rawConfig interface{}) (map[string]interface{}, error) {
	cfg, ok := rawConfig.(config.SlidingWindowLogConfig)
	if !ok {
		return nil, fmt.Errorf("expected SlidingWindowLogConfig, got %T", rawConfig)
	}

	windowSize := time.Duration(cfg.WindowSizeSeconds) * time.Second
	return map[string]interface{}{
		"key_prefix":         cfg.KeyPrefix,
		"ttl_buffer_seconds": cfg.TTLBufferSeconds,
		"window_size":        windowSize,
		"bucket_size":        cfg.BucketSize,
	}, nil
}
