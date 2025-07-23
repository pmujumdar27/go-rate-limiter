package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type TokenBucketConfig struct {
	BucketSize          int64
	RefillRatePerSecond int64
	KeyPrefix           string
	TTLBuffer           time.Duration
}

type TokenBucketRateLimiter struct {
	bucketSize          int64
	refillRatePerSecond int64
	redisClient         *redis.Client
	keyPrefix           string
	ttlBuffer           int64
}

func NewTokenBucketRateLimiter(config TokenBucketConfig, redisClient *redis.Client) (*TokenBucketRateLimiter, error) {
	if config.BucketSize <= 0 || config.RefillRatePerSecond <= 0 || redisClient == nil {
		return nil, errors.New("invalid configuration")
	}

	ttlBuffer := config.TTLBuffer
	if ttlBuffer <= 0 {
		ttlBuffer = 60 * time.Second
	}

	return &TokenBucketRateLimiter{
		bucketSize:          config.BucketSize,
		refillRatePerSecond: config.RefillRatePerSecond,
		redisClient:         redisClient,
		keyPrefix:           config.KeyPrefix,
		ttlBuffer:           int64(ttlBuffer.Seconds()),
	}, nil
}

func (tb *TokenBucketRateLimiter) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	redisKey := fmt.Sprintf("%s:%s", tb.keyPrefix, key)

	currentTimestampNanos := timestamp.UnixNano()

	script := `
		local key = KEYS[1]
		local bucket_size = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local current_time_nanos = tonumber(ARGV[3])
		local ttl_buffer_seconds = tonumber(ARGV[4])
		
		local bucket_data = redis.call('HMGET', key, 'tokens', 'last_refill_time_nanos')
		local current_tokens = bucket_size
		local last_refill_time_nanos = current_time_nanos
		
		if bucket_data[1] then
			current_tokens = tonumber(bucket_data[1])
		end
		
		if bucket_data[2] then
			last_refill_time_nanos = tonumber(bucket_data[2])
		end
		
		local time_since_last_refill_seconds = (current_time_nanos - last_refill_time_nanos) / 1e9
		
		local tokens_to_refill = time_since_last_refill_seconds * refill_rate
		
		current_tokens = math.min(bucket_size, current_tokens + tokens_to_refill)
		
		if current_tokens < 1 then
			local tokens_needed = 1 - current_tokens
			local seconds_until_token = tokens_needed / refill_rate
			local next_token_time_nanos = current_time_nanos + (seconds_until_token * 1e9)
			
			redis.call('HMSET', key, 
				'tokens', current_tokens,
				'last_refill_time_nanos', current_time_nanos)
			
					local ttl_seconds = math.max(3600, bucket_size / refill_rate + ttl_buffer_seconds)
		redis.call('EXPIRE', key, ttl_seconds)
			
			return {0, current_tokens, next_token_time_nanos}
		end
		
		local remaining_tokens = current_tokens - 1
		
		redis.call('HMSET', key, 
			'tokens', remaining_tokens,
			'last_refill_time_nanos', current_time_nanos)
		
		local ttl_seconds = math.max(3600, bucket_size / refill_rate + ttl_buffer_seconds)
		redis.call('EXPIRE', key, ttl_seconds)
		
		local tokens_to_full = bucket_size - remaining_tokens
		local seconds_to_full = tokens_to_full / refill_rate
		local full_time_nanos = current_time_nanos + (seconds_to_full * 1e9)
		
		return {1, remaining_tokens, full_time_nanos}
	`

	result, err := tb.redisClient.Eval(ctx, script, []string{redisKey},
		tb.bucketSize, tb.refillRatePerSecond, currentTimestampNanos, tb.ttlBuffer).Result()

	if err != nil {
		return RateLimitResponse{
			Err: err,
		}, err
	}

	resultArray := result.([]interface{})
	allowed := resultArray[0].(int64) == 1
	tokens := resultArray[1].(int64)
	timeNanos := resultArray[2].(int64)

	if allowed {
		remainingTokens := tokens
		fullTime := time.Unix(0, timeNanos)

		return RateLimitResponse{
			Allowed: true,
			Metadata: map[string]interface{}{
				"remaining_tokens": remainingTokens,
				"bucket_size":      tb.bucketSize,
				"refill_rate":      tb.refillRatePerSecond,
				"bucket_full_time": fullTime,
			},
		}, nil
	} else {
		currentTokens := tokens
		nextTokenTime := time.Unix(0, timeNanos)
		retryAfter := nextTokenTime.Sub(timestamp)

		return RateLimitResponse{
			Allowed: false,
			Metadata: map[string]interface{}{
				"current_tokens":   currentTokens,
				"remaining_tokens": 0,
				"bucket_size":      tb.bucketSize,
				"refill_rate":      tb.refillRatePerSecond,
				"next_token_time":  nextTokenTime,
				"retry_after":      retryAfter,
			},
		}, nil
	}
}

func (tb *TokenBucketRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:%s", tb.keyPrefix, key)

	_, err := tb.redisClient.Del(ctx, redisKey).Result()
	if err != nil {
		return err
	}

	return nil
}
