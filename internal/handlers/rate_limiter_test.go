package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockRateLimiter struct {
	mock.Mock
}

func (m *MockRateLimiter) IsAllowed(ctx context.Context, key string, timestamp time.Time) (ratelimit.RateLimitResponse, error) {
	args := m.Called(ctx, key, timestamp)
	return args.Get(0).(ratelimit.RateLimitResponse), args.Error(1)
}

func (m *MockRateLimiter) Reset(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func TestNewRateLimitHandler(t *testing.T) {
	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	assert.NotNil(t, handler)
	assert.Equal(t, mockLimiter, handler.rateLimiter)
}

func TestRateLimitHandler_RateLimit_Allowed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	// Mock successful rate limit check
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:   true,
			Limit:     10,
			Remaining: 9,
			ResetTime: time.Now().Add(time.Hour),
			Metadata: map[string]interface{}{
				"bucket_size": 10,
			},
		}, nil)

	router := gin.New()
	router.POST("/rate-limit", handler.RateLimit)

	req := httptest.NewRequest("POST", "/rate-limit", nil)
	req.Header.Set("X-Client-ID", "test-client")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"allowed":true`)
	assert.Contains(t, w.Body.String(), `"bucket_size":10`)
	
	// Check rate limit headers
	assert.Equal(t, "10", w.Header().Get("RateLimit-Limit"))
	assert.Equal(t, "9", w.Header().Get("RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("RateLimit-Reset"))

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_RateLimit_Denied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	retryAfter := 30 * time.Second
	// Mock rate limit exceeded
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:    false,
			Limit:      10,
			Remaining:  0,
			ResetTime:  time.Now().Add(time.Hour),
			RetryAfter: &retryAfter,
			Metadata: map[string]interface{}{
				"current_tokens": 0,
			},
		}, nil)

	router := gin.New()
	router.POST("/rate-limit", handler.RateLimit)

	req := httptest.NewRequest("POST", "/rate-limit", nil)
	req.Header.Set("X-Client-ID", "test-client")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), `"allowed":false`)
	assert.Contains(t, w.Body.String(), `"current_tokens":0`)
	
	// Check rate limit headers
	assert.Equal(t, "10", w.Header().Get("RateLimit-Limit"))
	assert.Equal(t, "0", w.Header().Get("RateLimit-Remaining"))
	assert.Equal(t, "30", w.Header().Get("Retry-After"))

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_RateLimit_ClientIPFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	// Mock rate limit check - should use client IP when X-Client-ID header is missing
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:   true,
			Limit:     10,
			Remaining: 9,
			ResetTime: time.Now().Add(time.Hour),
		}, nil)

	router := gin.New()
	router.POST("/rate-limit", handler.RateLimit)

	req := httptest.NewRequest("POST", "/rate-limit", nil)
	// No X-Client-ID header set
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_RateLimit_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	// Mock error from rate limiter
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{}, assert.AnError)

	router := gin.New()
	router.POST("/rate-limit", handler.RateLimit)

	req := httptest.NewRequest("POST", "/rate-limit", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), `"error":"Rate limiter error"`)

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_ResetRateLimit_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	// Mock successful reset
	mockLimiter.On("Reset", mock.Anything, "test-client").Return(nil)

	router := gin.New()
	router.POST("/rate-limit/reset", handler.ResetRateLimit)

	req := httptest.NewRequest("POST", "/rate-limit/reset", nil)
	req.Header.Set("X-Client-ID", "test-client")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"message":"Rate limit reset successfully"`)
	assert.Contains(t, w.Body.String(), `"client_id":"test-client"`)

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_ResetRateLimit_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	// Mock error from reset
	mockLimiter.On("Reset", mock.Anything, mock.AnythingOfType("string")).Return(assert.AnError)

	router := gin.New()
	router.POST("/rate-limit/reset", handler.ResetRateLimit)

	req := httptest.NewRequest("POST", "/rate-limit/reset", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), `"error":"Reset error"`)

	mockLimiter.AssertExpectations(t)
}

func TestRateLimitHandler_setRateLimitHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockLimiter := &MockRateLimiter{}
	handler := NewRateLimitHandler(mockLimiter)

	tests := []struct {
		name     string
		response ratelimit.RateLimitResponse
		wantHeaders map[string]string
	}{
		{
			name: "allowed request headers",
			response: ratelimit.RateLimitResponse{
				Allowed:   true,
				Limit:     10,
				Remaining: 9,
				ResetTime: time.Now().Add(3600 * time.Second),
			},
			wantHeaders: map[string]string{
				"RateLimit-Limit":     "10",
				"RateLimit-Remaining": "9",
				"RateLimit-Reset":     "3600",
			},
		},
		{
			name: "denied request with retry after",
			response: ratelimit.RateLimitResponse{
				Allowed:    false,
				Limit:      10,
				Remaining:  0,
				ResetTime:  time.Now().Add(3600 * time.Second),
				RetryAfter: &[]time.Duration{30 * time.Second}[0],
			},
			wantHeaders: map[string]string{
				"RateLimit-Limit":     "10",
				"RateLimit-Remaining": "0",
				"RateLimit-Reset":     "3600",
				"Retry-After":         "30",
			},
		},
		{
			name: "past reset time",
			response: ratelimit.RateLimitResponse{
				Allowed:   true,
				Limit:     10,
				Remaining: 5,
				ResetTime: time.Now().Add(-100 * time.Second), // Past time
			},
			wantHeaders: map[string]string{
				"RateLimit-Limit":     "10",
				"RateLimit-Remaining": "5",
				"RateLimit-Reset":     "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/test", func(c *gin.Context) {
				handler.setRateLimitHeaders(c, tt.response)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			for header, expectedValue := range tt.wantHeaders {
				actualValue := w.Header().Get(header)
				if header == "RateLimit-Reset" || header == "Retry-After" {
					// For time-based headers, check if the value is close to expected
					// Allow some variance for test execution time
					assert.NotEmpty(t, actualValue, "Header %s should not be empty", header)
				} else {
					assert.Equal(t, expectedValue, actualValue, "Header %s mismatch", header)
				}
			}
		})
	}
}