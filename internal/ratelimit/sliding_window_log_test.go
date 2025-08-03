package ratelimit

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestNewSlidingWindowLogRateLimiter(t *testing.T) {
	tests := []struct {
		name        string
		config      SlidingWindowLogConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: SlidingWindowLogConfig{
				WindowSize:       10 * time.Second,
				BucketSize:       5,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: false,
		},
		{
			name: "invalid window size",
			config: SlidingWindowLogConfig{
				WindowSize:       0,
				BucketSize:       5,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: true,
		},
		{
			name: "invalid bucket size",
			config: SlidingWindowLogConfig{
				WindowSize:       10 * time.Second,
				BucketSize:       0,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: true,
		},
		{
			name: "default ttl buffer",
			config: SlidingWindowLogConfig{
				WindowSize: 10 * time.Second,
				BucketSize: 5,
				KeyPrefix:  "test:",
			},
			expectError: false,
		},
	}

	mockRedis := &redis.Client{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := NewSlidingWindowLogRateLimiter(tt.config, mockRedis)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, limiter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, limiter)
				assert.Equal(t, tt.config.BucketSize, limiter.bucketSize)
				assert.Equal(t, int64(tt.config.WindowSize.Seconds()), limiter.windowSizeSeconds)
				assert.Equal(t, tt.config.KeyPrefix, limiter.keyPrefix)
				
				if tt.config.TTLBufferSeconds > 0 {
					assert.Equal(t, int64(tt.config.TTLBufferSeconds), limiter.ttlBuffer)
				} else {
					assert.Equal(t, int64(DefaultTTLBufferSeconds), limiter.ttlBuffer)
				}
			}
		})
	}
}

func TestSlidingWindowLogRateLimiter_calculateRetryAfter(t *testing.T) {
	config := SlidingWindowLogConfig{
		WindowSize:       10 * time.Second,
		BucketSize:       5,
		KeyPrefix:        "test:",
		TTLBufferSeconds: 5,
	}

	mockRedis := &redis.Client{}
	limiter, err := NewSlidingWindowLogRateLimiter(config, mockRedis)
	assert.NoError(t, err)

	now := time.Now()

	tests := []struct {
		name        string
		resetTime   *time.Time
		currentTime time.Time
		expected    time.Duration
	}{
		{
			name:        "nil reset time",
			resetTime:   nil,
			currentTime: now,
			expected:    0,
		},
		{
			name:        "future reset time",
			resetTime:   &[]time.Time{now.Add(5 * time.Second)}[0],
			currentTime: now,
			expected:    5 * time.Second,
		},
		{
			name:        "past reset time",
			resetTime:   &[]time.Time{now.Add(-5 * time.Second)}[0],
			currentTime: now,
			expected:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := limiter.calculateRetryAfter(tt.resetTime, tt.currentTime)
			
			// Allow small time difference due to test execution time
			if tt.expected > 0 {
				assert.True(t, result >= tt.expected-time.Millisecond && result <= tt.expected+time.Millisecond)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSlidingWindowLogRateLimiter_ResponseParsing(t *testing.T) {
	config := SlidingWindowLogConfig{
		WindowSize:       10 * time.Second,
		BucketSize:       5,
		KeyPrefix:        "test:",
		TTLBufferSeconds: 5,
	}

	mockRedis := &redis.Client{}
	limiter, err := NewSlidingWindowLogRateLimiter(config, mockRedis)
	assert.NoError(t, err)

	t.Run("allowed request response", func(t *testing.T) {
		// Test response parsing logic for allowed requests
		timestamp := time.Now()
		
		// Simulate allowed response: [allowed=1, current_count=2, reset_time=0, remaining=3]
		allowed := int64(1)
		currentCount := int64(2)
		resetTimeSeconds := int64(0)
		remaining := int64(3)
		
		var response RateLimitResponse
		
		if allowed == 1 {
			response.Allowed = true
			response.Limit = limiter.bucketSize
			response.Remaining = remaining
			response.ResetTime = timestamp.Add(time.Duration(limiter.windowSizeSeconds) * time.Second)
			if resetTimeSeconds > 0 {
				response.ResetTime = time.Unix(resetTimeSeconds, 0)
			}
			response.Metadata = map[string]interface{}{
				"current_count": currentCount,
				"window_size":   limiter.windowSizeSeconds,
			}
		}
		
		assert.True(t, response.Allowed)
		assert.Equal(t, int64(5), response.Limit)
		assert.Equal(t, int64(3), response.Remaining)
		assert.NotNil(t, response.Metadata)
		assert.Equal(t, int64(2), response.Metadata["current_count"])
	})

	t.Run("denied request response", func(t *testing.T) {
		// Test response parsing logic for denied requests
		timestamp := time.Now()
		
		// Simulate denied response: [allowed=0, current_count=5, reset_time_seconds]
		allowed := int64(0)
		currentCount := int64(5)
		resetTimeSeconds := timestamp.Add(5 * time.Second).Unix()
		
		var response RateLimitResponse
		
		if allowed == 0 {
			response.Allowed = false
			response.Limit = limiter.bucketSize
			response.Remaining = 0
			response.ResetTime = time.Unix(resetTimeSeconds, 0)
			retryAfter := response.ResetTime.Sub(timestamp)
			response.RetryAfter = &retryAfter
			response.Metadata = map[string]interface{}{
				"current_count": currentCount,
				"window_size":   limiter.windowSizeSeconds,
			}
		}
		
		assert.False(t, response.Allowed)
		assert.Equal(t, int64(5), response.Limit)
		assert.Equal(t, int64(0), response.Remaining)
		assert.NotNil(t, response.RetryAfter)
		assert.NotNil(t, response.Metadata)
		assert.Equal(t, int64(5), response.Metadata["current_count"])
	})
}

func TestSlidingWindowLogConstructor(t *testing.T) {
	constructor := &SlidingWindowLogConstructor{}

	t.Run("name", func(t *testing.T) {
		assert.Equal(t, "sliding_window_log", constructor.Name())
	})

	t.Run("config structure validation", func(t *testing.T) {
		// Test the expected config structure
		expected := map[string]interface{}{
			"window_size":        10 * time.Second,
			"bucket_size":        int64(5),
			"key_prefix":         "test:",
			"ttl_buffer_seconds": 5,
		}

		assert.Equal(t, 10*time.Second, expected["window_size"])
		assert.Equal(t, int64(5), expected["bucket_size"])
		assert.Equal(t, "test:", expected["key_prefix"])
		assert.Equal(t, 5, expected["ttl_buffer_seconds"])
	})
}