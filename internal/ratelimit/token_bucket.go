package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pmujumdar27/go-rate-limiter/internal/config"
	"github.com/redis/go-redis/v9"
)

type TokenBucketConfig struct {
	BucketSize          int64
	RefillRatePerSecond int64
	KeyPrefix           string
	TTLBufferSeconds    int
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

	ttlBufferSeconds := config.TTLBufferSeconds
	if ttlBufferSeconds <= 0 {
		ttlBufferSeconds = DefaultTTLBufferSeconds
	}

	return &TokenBucketRateLimiter{
		bucketSize:          config.BucketSize,
		refillRatePerSecond: config.RefillRatePerSecond,
		redisClient:         redisClient,
		keyPrefix:           config.KeyPrefix,
		ttlBuffer:           int64(ttlBufferSeconds),
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
		
		local time_since_last_refill_seconds = (current_time_nanos - last_refill_time_nanos) / 1000000000 -- NanosecondsPerSecond
		
		local tokens_to_refill = time_since_last_refill_seconds * refill_rate
		
		current_tokens = math.min(bucket_size, current_tokens + tokens_to_refill)
		
		if current_tokens < 1 then
			local tokens_needed = 1 - current_tokens
			local seconds_until_token = tokens_needed / refill_rate
			local next_token_time_nanos = current_time_nanos + (seconds_until_token * 1000000000) -- NanosecondsPerSecond
			
			redis.call('HMSET', key, 
				'tokens', current_tokens,
				'last_refill_time_nanos', current_time_nanos)
			
			local ttl_seconds = math.max(60, bucket_size / refill_rate + ttl_buffer_seconds) -- MinimumTTLSeconds
			redis.call('EXPIRE', key, ttl_seconds)
			
			return {0, current_tokens, next_token_time_nanos}
		end
		
		local remaining_tokens = current_tokens - 1
		
		redis.call('HMSET', key, 
			'tokens', remaining_tokens,
			'last_refill_time_nanos', current_time_nanos)
		
		local ttl_seconds = math.max(60, bucket_size / refill_rate + ttl_buffer_seconds) -- MinimumTTLSeconds
		redis.call('EXPIRE', key, ttl_seconds)
		
		local tokens_to_full = bucket_size - remaining_tokens
		local seconds_to_full = tokens_to_full / refill_rate
		local full_time_nanos = current_time_nanos + (seconds_to_full * 1000000000) -- NanosecondsPerSecond
		
		return {1, remaining_tokens, full_time_nanos}
	`

	result, err := tb.redisClient.Eval(ctx, script, []string{redisKey},
		tb.bucketSize, tb.refillRatePerSecond, currentTimestampNanos, tb.ttlBuffer).Result()

	if err != nil {
		return RateLimitResponse{
			Err: err,
		}, err
	}

	resultArray, ok := result.([]interface{})
	if !ok || len(resultArray) < 3 {
		err = errors.New("invalid redis response from token bucket script")
		return RateLimitResponse{Err: err}, err
	}

	allowed, err := getInt64FromResult(resultArray[0])
	if err != nil {
		err = fmt.Errorf("failed to parse allowed flag: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	tokens, err := getInt64FromResult(resultArray[1])
	if err != nil {
		err = fmt.Errorf("failed to parse tokens: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	timeNanos, err := getInt64FromResult(resultArray[2])
	if err != nil {
		err = fmt.Errorf("failed to parse time: %w", err)
		return RateLimitResponse{Err: err}, err
	}

	metadata := map[string]interface{}{
		"bucket_size": tb.bucketSize,
		"refill_rate": tb.refillRatePerSecond,
	}

	if allowed == 1 {
		remainingTokens := tokens
		fullTime := time.Unix(0, timeNanos)
		metadata["bucket_full_time"] = fullTime

		return RateLimitResponse{
			Allowed:   true,
			Limit:     tb.bucketSize,
			Remaining: remainingTokens,
			ResetTime: fullTime,
			Metadata:  metadata,
		}, nil
	}

	currentTokens := tokens
	nextTokenTime := time.Unix(0, timeNanos)
	retryAfter := nextTokenTime.Sub(timestamp)
	metadata["current_tokens"] = currentTokens
	metadata["next_token_time"] = nextTokenTime

	return RateLimitResponse{
		Allowed:    false,
		Limit:      tb.bucketSize,
		Remaining:  0,
		ResetTime:  nextTokenTime,
		RetryAfter: &retryAfter,
		Metadata:   metadata,
	}, nil
}

func (tb *TokenBucketRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("%s:%s", tb.keyPrefix, key)

	_, err := tb.redisClient.Del(ctx, redisKey).Result()
	if err != nil {
		return err
	}

	return nil
}

type TokenBucketConstructor struct{}

func (c *TokenBucketConstructor) Name() string {
	return "token_bucket"
}

func (c *TokenBucketConstructor) NewFromConfig(config map[string]interface{}, redisClient *redis.Client) (RateLimiter, error) {
	bucketSize, err := getInt64Config(config, "bucket_size")
	if err != nil {
		return nil, fmt.Errorf("token bucket strategy: %w", err)
	}
	refillRate, err := getInt64Config(config, "refill_rate_per_second")
	if err != nil {
		return nil, fmt.Errorf("token bucket strategy: %w", err)
	}
	keyPrefix, err := getStringConfig(config, "key_prefix")
	if err != nil {
		return nil, fmt.Errorf("token bucket strategy: %w", err)
	}
	ttlBuffer, err := getIntConfig(config, "ttl_buffer_seconds")
	if err != nil {
		return nil, fmt.Errorf("token bucket strategy: %w", err)
	}

	tokenBucketConfig := TokenBucketConfig{
		BucketSize:          bucketSize,
		RefillRatePerSecond: refillRate,
		KeyPrefix:           keyPrefix,
		TTLBufferSeconds:    ttlBuffer,
	}
	return NewTokenBucketRateLimiter(tokenBucketConfig, redisClient)
}

func (c *TokenBucketConstructor) ConvertConfig(rawConfig interface{}) (map[string]interface{}, error) {
	cfg, ok := rawConfig.(config.TokenBucketConfig)
	if !ok {
		return nil, fmt.Errorf("expected TokenBucketConfig, got %T", rawConfig)
	}

	return map[string]interface{}{
		"key_prefix":             cfg.KeyPrefix,
		"ttl_buffer_seconds":     cfg.TTLBufferSeconds,
		"bucket_size":            cfg.BucketSize,
		"refill_rate_per_second": cfg.RefillRatePerSecond,
	}, nil
}
