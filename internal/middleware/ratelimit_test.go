package middleware

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

func TestRateLimitMiddleware_Allowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	mockLimiter := new(MockRateLimiter)
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:   true,
			Limit:     10,
			Remaining: 9,
			ResetTime: time.Now().Add(time.Hour),
		}, nil)

	router := gin.New()
	router.GET("/test", RateLimit(mockLimiter), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "success")
	assert.Equal(t, "10", w.Header().Get("RateLimit-Limit"))
	assert.Equal(t, "9", w.Header().Get("RateLimit-Remaining"))
	
	mockLimiter.AssertExpectations(t)
}

func TestRateLimitMiddleware_Denied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	retryAfter := 30 * time.Second
	mockLimiter := new(MockRateLimiter)
	mockLimiter.On("IsAllowed", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:    false,
			Limit:      10,
			Remaining:  0,
			ResetTime:  time.Now().Add(time.Hour),
			RetryAfter: &retryAfter,
		}, nil)

	router := gin.New()
	router.GET("/test", RateLimit(mockLimiter), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "Too many requests")
	assert.Equal(t, "10", w.Header().Get("RateLimit-Limit"))
	assert.Equal(t, "0", w.Header().Get("RateLimit-Remaining"))
	assert.Equal(t, "30", w.Header().Get("Retry-After"))
	
	mockLimiter.AssertExpectations(t)
}

func TestRateLimitMiddleware_CustomKeyExtractor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	
	mockLimiter := new(MockRateLimiter)
	mockLimiter.On("IsAllowed", mock.Anything, "custom-key", mock.Anything).Return(
		ratelimit.RateLimitResponse{
			Allowed:   true,
			Limit:     10,
			Remaining: 9,
			ResetTime: time.Now().Add(time.Hour),
		}, nil)

	config := &RateLimitConfig{
		KeyExtractor: func(c *gin.Context) string {
			return "custom-key"
		},
	}

	router := gin.New()
	router.GET("/test", RateLimit(mockLimiter, config), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	mockLimiter.AssertExpectations(t)
}