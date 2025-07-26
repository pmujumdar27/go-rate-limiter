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

type SlidingWindowCounterConfig struct {
	WindowSize       time.Duration
	BucketSize       int64
	KeyPrefix        string
	TTLBufferSeconds int
}

type SlidingWindowCounterRateLimiter struct {
	windowSizeNanos int64
	redisClient     *redis.Client
	keyPrefix       string
	bucketSize      int64
	ttlBuffer       int64
}

func NewSlidingWindowCounterRateLimiter(config SlidingWindowCounterConfig, redisClient *redis.Client) (*SlidingWindowCounterRateLimiter, error) {
	if config.WindowSize <= 0 || config.BucketSize <= 0 || redisClient == nil {
		return nil, errors.New("invalid configuration")
	}

	ttlBufferSeconds := config.TTLBufferSeconds
	if ttlBufferSeconds <= 0 {
		ttlBufferSeconds = DefaultTTLBufferSeconds
	}

	return &SlidingWindowCounterRateLimiter{
		windowSizeNanos: int64(config.WindowSize.Nanoseconds()),
		redisClient:     redisClient,
		keyPrefix:       config.KeyPrefix,
		bucketSize:      config.BucketSize,
		ttlBuffer:       int64(ttlBufferSeconds),
	}, nil
}

func (swc *SlidingWindowCounterRateLimiter) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	start := time.Now()
	defer func() {
		metrics.RateLimitDuration.WithLabelValues("sliding_window_counter").Observe(time.Since(start).Seconds())
	}()

	redisKey := fmt.Sprintf("%s:%s", swc.keyPrefix, key)
	currentTimestampNanos := timestamp.UnixNano()
	currentWindowStart := (currentTimestampNanos / swc.windowSizeNanos) * swc.windowSizeNanos
	previousWindowStart := currentWindowStart - swc.windowSizeNanos

	timeIntoWindow := currentTimestampNanos - currentWindowStart
	windowProgress := float64(timeIntoWindow) / float64(swc.windowSizeNanos)
	if windowProgress > 1.0 {
		windowProgress = 1.0
	}

	script := `
		local key = KEYS[1]
		local current_window_start = tonumber(ARGV[1])
		local previous_window_start = tonumber(ARGV[2])
		local bucket_size = tonumber(ARGV[3])
		local window_size_nanos = tonumber(ARGV[4])
		local ttl_seconds = tonumber(ARGV[5])
		local window_progress = tonumber(ARGV[6])

		local current_window_key = key .. ':current'
		local previous_window_key = key .. ':previous'

		local current_count = 0
		local previous_count = 0

		local current_window_data = redis.call('HMGET', current_window_key, 'count', 'window_start')
		if current_window_data[1] and current_window_data[2] then
			local stored_window_start = tonumber(current_window_data[2])
			if stored_window_start == current_window_start then
				current_count = tonumber(current_window_data[1])
			elseif stored_window_start == previous_window_start then
				previous_count = tonumber(current_window_data[1])
			end
		end

		if previous_count == 0 then
			local previous_window_data = redis.call('HMGET', previous_window_key, 'count', 'window_start')
			if previous_window_data[1] and previous_window_data[2] and tonumber(previous_window_data[2]) == previous_window_start then
				previous_count = tonumber(previous_window_data[1])
			end
		end

		local previous_window_weight = 1 - window_progress
		local weighted_count = math.floor(current_count + (previous_count * previous_window_weight))

		if weighted_count >= bucket_size then
			local reset_time_nanos = current_window_start + window_size_nanos
			return {0, weighted_count, reset_time_nanos, current_count, previous_count}
		end

		local new_current_count = current_count + 1
		redis.call('HMSET', current_window_key, 'count', new_current_count, 'window_start', current_window_start)
		redis.call('EXPIRE', current_window_key, ttl_seconds)

		redis.call('HMSET', previous_window_key, 'count', previous_count, 'window_start', previous_window_start)
		redis.call('EXPIRE', previous_window_key, ttl_seconds)

		local remaining_requests = math.max(0, bucket_size - weighted_count - 1)
		return {1, weighted_count + 1, 0, new_current_count, previous_count, remaining_requests}
	`

	ttlSeconds := (swc.windowSizeNanos/NanosecondsPerSecond)*2 + swc.ttlBuffer

	redisStart := time.Now()
	result, err := swc.redisClient.Eval(ctx, script, []string{redisKey},
		currentWindowStart, previousWindowStart, swc.bucketSize, swc.windowSizeNanos, ttlSeconds, windowProgress).Result()
	metrics.RedisOperationDuration.WithLabelValues("eval").Observe(time.Since(redisStart).Seconds())

	if err != nil {
		metrics.RedisOperations.WithLabelValues("eval", "error").Inc()
		return RateLimitResponse{Err: err}, err
	}
	metrics.RedisOperations.WithLabelValues("eval", "success").Inc()

	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) < 5 {
		err = errors.New("invalid redis response from rate limit script")
		return RateLimitResponse{Err: err}, err
	}

	allowed, err := getInt64FromResult(resultArray[0])
	if err != nil {
		err = fmt.Errorf("failed to parse allowed flag: %w", err)
		return RateLimitResponse{Err: err}, err
	}
	
	weightedCount, err := getInt64FromResult(resultArray[1])
	if err != nil {
		err = fmt.Errorf("failed to parse weighted count: %w", err)
		return RateLimitResponse{Err: err}, err
	}
	
	resetTimeNanos, err := getInt64FromResult(resultArray[2])
	if err != nil {
		err = fmt.Errorf("failed to parse reset time: %w", err)
		return RateLimitResponse{Err: err}, err
	}
	
	currentCount, err := getInt64FromResult(resultArray[3])
	if err != nil {
		err = fmt.Errorf("failed to parse current count: %w", err)
		return RateLimitResponse{Err: err}, err
	}
	
	previousCount, err := getInt64FromResult(resultArray[4])
	if err != nil {
		err = fmt.Errorf("failed to parse previous count: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	metadata := map[string]interface{}{
		"weighted_count":  weightedCount,
		"current_count":   currentCount,
		"previous_count":  previousCount,
		"window_progress": windowProgress,
		"window_size":     swc.windowSizeNanos / NanosecondsPerSecond,
	}

	resetTime := time.Unix(0, currentWindowStart+swc.windowSizeNanos)
	if resetTimeNanos > 0 {
		resetTime = time.Unix(0, resetTimeNanos)
	}

	if allowed == 1 {
		metrics.RateLimitRequests.WithLabelValues("sliding_window_counter", "allowed").Inc()
		remainingRequests := int64(0)
		if len(resultArray) > 5 {
			if remaining, err := getInt64FromResult(resultArray[5]); err == nil {
				remainingRequests = remaining
			}
		}

		return RateLimitResponse{
			Allowed:   true,
			Limit:     swc.bucketSize,
			Remaining: remainingRequests,
			ResetTime: resetTime,
			Metadata:  metadata,
		}, nil
	}

	metrics.RateLimitRequests.WithLabelValues("sliding_window_counter", "denied").Inc()
	retryAfter := swc.calculateRetryAfter(currentCount, previousCount, currentWindowStart, currentTimestampNanos)

	return RateLimitResponse{
		Allowed:    false,
		Limit:      swc.bucketSize,
		Remaining:  0,
		ResetTime:  resetTime,
		RetryAfter: &retryAfter,
		Metadata:   metadata,
	}, nil
}

func (swc *SlidingWindowCounterRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:%s", swc.keyPrefix, key)
	currentWindowKey := fmt.Sprintf("%s:current", redisKey)
	previousWindowKey := fmt.Sprintf("%s:previous", redisKey)

	_, err := swc.redisClient.Del(ctx, currentWindowKey, previousWindowKey).Result()
	return err
}

func (swc *SlidingWindowCounterRateLimiter) calculateRetryAfter(currentCount, previousCount, currentWindowStart, currentTimestamp int64) time.Duration {
	if previousCount == 0 {
		retryAfterNanos := (currentWindowStart + swc.windowSizeNanos) - currentTimestamp
		return time.Duration(retryAfterNanos)
	}

	// currentCount + (1 - windowProgress) * previousCount = bucketSize
	// windowProgress = 1 - (bucketSize - currentCount) / previousCount
	requiredWindowProgress := 1.0 - float64(swc.bucketSize-currentCount)/float64(previousCount)

	// If required progress is >= 1, we need to wait until next window
	if requiredWindowProgress >= 1.0 {
		retryAfterNanos := (currentWindowStart + swc.windowSizeNanos) - currentTimestamp
		return time.Duration(retryAfterNanos)
	}

	futureTimestamp := currentWindowStart + int64(requiredWindowProgress*float64(swc.windowSizeNanos))

	retryAfter := futureTimestamp - currentTimestamp

	return time.Duration(retryAfter)
}

type SlidingWindowCounterConstructor struct{}

func (c *SlidingWindowCounterConstructor) Name() string {
	return "sliding_window_counter"
}

func (c *SlidingWindowCounterConstructor) NewFromConfig(config map[string]interface{}, redisClient *redis.Client) (RateLimiter, error) {
	windowSize, err := getDurationConfig(config, "window_size")
	if err != nil {
		return nil, fmt.Errorf("sliding window counter strategy: %w", err)
	}
	bucketSize, err := getInt64Config(config, "bucket_size")
	if err != nil {
		return nil, fmt.Errorf("sliding window counter strategy: %w", err)
	}
	keyPrefix, err := getStringConfig(config, "key_prefix")
	if err != nil {
		return nil, fmt.Errorf("sliding window counter strategy: %w", err)
	}
	ttlBuffer, err := getIntConfig(config, "ttl_buffer_seconds")
	if err != nil {
		return nil, fmt.Errorf("sliding window counter strategy: %w", err)
	}
	
	slidingWindowCounterConfig := SlidingWindowCounterConfig{
		WindowSize:       windowSize,
		BucketSize:       bucketSize,
		KeyPrefix:        keyPrefix,
		TTLBufferSeconds: ttlBuffer,
	}
	return NewSlidingWindowCounterRateLimiter(slidingWindowCounterConfig, redisClient)
}

func (c *SlidingWindowCounterConstructor) ConvertConfig(rawConfig interface{}) (map[string]interface{}, error) {
	cfg, ok := rawConfig.(config.SlidingWindowCounterConfig)
	if !ok {
		return nil, fmt.Errorf("expected SlidingWindowCounterConfig, got %T", rawConfig)
	}
	
	windowSize := time.Duration(cfg.WindowSizeSeconds) * time.Second
	return map[string]interface{}{
		"key_prefix":         cfg.KeyPrefix,
		"ttl_buffer_seconds": cfg.TTLBufferSeconds,
		"window_size":        windowSize,
		"bucket_size":        cfg.BucketSize,
	}, nil
}
