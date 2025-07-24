package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type SlidingWindowLogConfig struct {
	WindowSize time.Duration
	BucketSize int64
	KeyPrefix  string
	TTLBuffer  time.Duration
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

	ttlBuffer := config.TTLBuffer
	if ttlBuffer <= 0 {
		ttlBuffer = 60 * time.Second
	}

	return &SlidingWindowLogRateLimiter{
		windowSizeSeconds: int64(config.WindowSize.Seconds()),
		redisClient:       redisClient,
		keyPrefix:         config.KeyPrefix,
		bucketSize:        config.BucketSize,
		ttlBuffer:         int64(ttlBuffer.Seconds()),
	}, nil
}

func (swl *SlidingWindowLogRateLimiter) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	redisKey := fmt.Sprintf("%s:%s", swl.keyPrefix, key)

	currentTimestampNanos := timestamp.UnixNano()
	windowStartNanos := currentTimestampNanos - (swl.windowSizeSeconds * 1e9)

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
				reset_time_seconds = (oldest_timestamp_nanos + (window_size_seconds * 1e9)) / 1e9
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

	result, err := swl.redisClient.Eval(ctx, script, []string{redisKey},
		windowStartNanos, currentTimestampNanos, swl.bucketSize, swl.windowSizeSeconds, swl.ttlBuffer).Result()

	if err != nil {
		return RateLimitResponse{
			Err: err,
		}, err
	}

	resultArray := result.([]interface{})
	allowed := resultArray[0].(int64) == 1
	currentCount := resultArray[1].(int64)
	resetTimeSeconds := resultArray[2].(int64)

	if allowed {
		remainingRequests := resultArray[3].(int64)
		return RateLimitResponse{
			Allowed: true,
			Metadata: map[string]interface{}{
				"remaining_requests": remainingRequests,
				"current_count":      currentCount,
				"window_size":        swl.windowSizeSeconds,
			},
		}, nil
	} else {
		var resetTime *time.Time
		if resetTimeSeconds > 0 {
			rt := time.Unix(resetTimeSeconds, 0)
			resetTime = &rt
		}

		return RateLimitResponse{
			Allowed: false,
			Metadata: map[string]interface{}{
				"remaining_requests": 0,
				"current_count":      currentCount,
				"limit":              swl.bucketSize,
				"window_size":        swl.windowSizeSeconds,
				"reset_time":         resetTime,
				"retry_after_s":      swl.calculateRetryAfter(resetTime, timestamp).Seconds(),
			},
		}, nil
	}
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
