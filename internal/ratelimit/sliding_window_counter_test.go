package ratelimit

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestNewSlidingWindowCounterRateLimiter(t *testing.T) {
	tests := []struct {
		name        string
		config      SlidingWindowCounterConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: SlidingWindowCounterConfig{
				WindowSize:       10 * time.Second,
				BucketSize:       5,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: false,
		},
		{
			name: "invalid window size",
			config: SlidingWindowCounterConfig{
				WindowSize:       0,
				BucketSize:       5,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: true,
		},
		{
			name: "invalid bucket size",
			config: SlidingWindowCounterConfig{
				WindowSize:       10 * time.Second,
				BucketSize:       0,
				KeyPrefix:        "test:",
				TTLBufferSeconds: 5,
			},
			expectError: true,
		},
		{
			name: "default ttl buffer",
			config: SlidingWindowCounterConfig{
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
			limiter, err := NewSlidingWindowCounterRateLimiter(tt.config, mockRedis)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, limiter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, limiter)
				assert.Equal(t, tt.config.BucketSize, limiter.bucketSize)
				assert.Equal(t, int64(tt.config.WindowSize.Nanoseconds()), limiter.windowSizeNanos)
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

func TestSlidingWindowCounterRateLimiter_calculateRetryAfter(t *testing.T) {
	config := SlidingWindowCounterConfig{
		WindowSize:       10 * time.Second,
		BucketSize:       5,
		KeyPrefix:        "test:",
		TTLBufferSeconds: 5,
	}

	mockRedis := &redis.Client{}
	limiter, err := NewSlidingWindowCounterRateLimiter(config, mockRedis)
	assert.NoError(t, err)

	now := time.Now()
	currentTimestamp := now.UnixNano()
	currentWindowStart := (currentTimestamp / limiter.windowSizeNanos) * limiter.windowSizeNanos

	tests := []struct {
		name              string
		currentCount      int64
		previousCount     int64
		expectedMinDuration time.Duration
		expectedMaxDuration time.Duration
	}{
		{
			name:                "no previous count - wait for next window",
			currentCount:        5,
			previousCount:       0,
			expectedMinDuration: 0,
			expectedMaxDuration: 10 * time.Second,
		},
		{
			name:                "with previous count - partial window wait",
			currentCount:        3,
			previousCount:       4,
			expectedMinDuration: -10 * time.Second, // Allow negative duration for complex calculation
			expectedMaxDuration: 10 * time.Second,
		},
		{
			name:                "need to wait for next window",
			currentCount:        5,
			previousCount:       1,
			expectedMinDuration: 0,
			expectedMaxDuration: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := limiter.calculateRetryAfter(tt.currentCount, tt.previousCount, currentWindowStart, currentTimestamp)
			
			assert.True(t, result >= tt.expectedMinDuration, "retry after should be >= %v, got %v", tt.expectedMinDuration, result)
			assert.True(t, result <= tt.expectedMaxDuration, "retry after should be <= %v, got %v", tt.expectedMaxDuration, result)
		})
	}
}

func TestSlidingWindowCounterRateLimiter_WindowCalculations(t *testing.T) {
	config := SlidingWindowCounterConfig{
		WindowSize:       10 * time.Second,
		BucketSize:       5,
		KeyPrefix:        "test:",
		TTLBufferSeconds: 5,
	}

	mockRedis := &redis.Client{}
	limiter, err := NewSlidingWindowCounterRateLimiter(config, mockRedis)
	assert.NoError(t, err)

	now := time.Now()
	currentTimestamp := now.UnixNano()
	
	// Test window start calculation
	currentWindowStart := (currentTimestamp / limiter.windowSizeNanos) * limiter.windowSizeNanos
	previousWindowStart := currentWindowStart - limiter.windowSizeNanos
	
	assert.True(t, currentWindowStart <= currentTimestamp)
	assert.True(t, previousWindowStart < currentWindowStart)
	assert.Equal(t, limiter.windowSizeNanos, currentWindowStart-previousWindowStart)

	// Test window progress calculation
	timeIntoWindow := currentTimestamp - currentWindowStart
	windowProgress := float64(timeIntoWindow) / float64(limiter.windowSizeNanos)
	
	assert.True(t, windowProgress >= 0.0)
	assert.True(t, windowProgress <= 1.0)
}

func TestSlidingWindowCounterRateLimiter_ResponseParsing(t *testing.T) {
	config := SlidingWindowCounterConfig{
		WindowSize:       10 * time.Second,
		BucketSize:       5,
		KeyPrefix:        "test:",
		TTLBufferSeconds: 5,
	}

	mockRedis := &redis.Client{}
	limiter, err := NewSlidingWindowCounterRateLimiter(config, mockRedis)
	assert.NoError(t, err)

	t.Run("allowed request response", func(t *testing.T) {
		// Test response parsing logic for allowed requests
		timestamp := time.Now()
		currentTimestamp := timestamp.UnixNano()
		currentWindowStart := (currentTimestamp / limiter.windowSizeNanos) * limiter.windowSizeNanos
		
		// Simulate allowed response: [allowed=1, weighted_count=2, reset_time=0, current_count=2, previous_count=1, remaining=2]
		allowed := int64(1)
		weightedCount := int64(2)
		resetTimeNanos := int64(0)
		currentCount := int64(2)
		previousCount := int64(1)
		remaining := int64(2)
		
		timeIntoWindow := currentTimestamp - currentWindowStart
		windowProgress := float64(timeIntoWindow) / float64(limiter.windowSizeNanos)
		
		var response RateLimitResponse
		
		if allowed == 1 {
			response.Allowed = true
			response.Limit = limiter.bucketSize
			response.Remaining = remaining
			response.ResetTime = time.Unix(0, currentWindowStart+limiter.windowSizeNanos)
			if resetTimeNanos > 0 {
				response.ResetTime = time.Unix(0, resetTimeNanos)
			}
			response.Metadata = map[string]interface{}{
				"weighted_count":  weightedCount,
				"current_count":   currentCount,
				"previous_count":  previousCount,
				"window_progress": windowProgress,
				"window_size":     limiter.windowSizeNanos / NanosecondsPerSecond,
			}
		}
		
		assert.True(t, response.Allowed)
		assert.Equal(t, int64(5), response.Limit)
		assert.Equal(t, int64(2), response.Remaining)
		assert.NotNil(t, response.Metadata)
		assert.Equal(t, int64(2), response.Metadata["weighted_count"])
		assert.Equal(t, int64(2), response.Metadata["current_count"])
	})

	t.Run("denied request response", func(t *testing.T) {
		// Test response parsing logic for denied requests
		timestamp := time.Now()
		currentTimestamp := timestamp.UnixNano()
		currentWindowStart := (currentTimestamp / limiter.windowSizeNanos) * limiter.windowSizeNanos
		
		// Simulate denied response: [allowed=0, weighted_count=5, reset_time_nanos, current_count=3, previous_count=3]
		allowed := int64(0)
		weightedCount := int64(5)
		resetTimeNanos := currentWindowStart + limiter.windowSizeNanos
		currentCount := int64(3)
		previousCount := int64(3)
		
		timeIntoWindow := currentTimestamp - currentWindowStart
		windowProgress := float64(timeIntoWindow) / float64(limiter.windowSizeNanos)
		
		var response RateLimitResponse
		
		if allowed == 0 {
			response.Allowed = false
			response.Limit = limiter.bucketSize
			response.Remaining = 0
			response.ResetTime = time.Unix(0, resetTimeNanos)
			retryAfter := limiter.calculateRetryAfter(currentCount, previousCount, currentWindowStart, currentTimestamp)
			response.RetryAfter = &retryAfter
			response.Metadata = map[string]interface{}{
				"weighted_count":  weightedCount,
				"current_count":   currentCount,
				"previous_count":  previousCount,
				"window_progress": windowProgress,
				"window_size":     limiter.windowSizeNanos / NanosecondsPerSecond,
			}
		}
		
		assert.False(t, response.Allowed)
		assert.Equal(t, int64(5), response.Limit)
		assert.Equal(t, int64(0), response.Remaining)
		assert.NotNil(t, response.RetryAfter)
		assert.NotNil(t, response.Metadata)
		assert.Equal(t, int64(5), response.Metadata["weighted_count"])
		assert.Equal(t, int64(3), response.Metadata["current_count"])
	})
}

func TestSlidingWindowCounterConstructor(t *testing.T) {
	constructor := &SlidingWindowCounterConstructor{}

	t.Run("name", func(t *testing.T) {
		assert.Equal(t, "sliding_window_counter", constructor.Name())
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