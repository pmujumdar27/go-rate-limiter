package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/pmujumdar27/go-rate-limiter/internal/metrics"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockStrategyConstructor struct {
	mock.Mock
}

func (m *MockStrategyConstructor) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStrategyConstructor) NewFromConfig(config map[string]interface{}, redisClient *redis.Client) (RateLimiter, error) {
	args := m.Called(config, redisClient)
	return args.Get(0).(RateLimiter), args.Error(1)
}

func (m *MockStrategyConstructor) ConvertConfig(rawConfig interface{}) (map[string]interface{}, error) {
	args := m.Called(rawConfig)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

type MockRateLimiterForFactory struct {
	mock.Mock
}

func (m *MockRateLimiterForFactory) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	args := m.Called(ctx, key, timestamp)
	return args.Get(0).(RateLimitResponse), args.Error(1)
}

func (m *MockRateLimiterForFactory) Reset(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func TestNewFactory(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	assert.NotNil(t, factory)
	assert.Equal(t, mockRedis, factory.redisClient)
	assert.NotNil(t, factory.strategies)
	assert.NotNil(t, factory.metricsCollector)

	// Check that default strategies are registered
	strategies := factory.GetAvailableStrategies()
	assert.Contains(t, strategies, "token_bucket")
	assert.Contains(t, strategies, "sliding_window_log")
	assert.Contains(t, strategies, "sliding_window_counter")
	assert.Len(t, strategies, 3)
}

func TestFactory_RegisterStrategy(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	mockConstructor := &MockStrategyConstructor{}
	mockConstructor.On("Name").Return("test_strategy")

	factory.RegisterStrategy(mockConstructor)

	strategies := factory.GetAvailableStrategies()
	assert.Contains(t, strategies, "test_strategy")
	
	mockConstructor.AssertExpectations(t)
}

func TestFactory_CreateRateLimiter_Success(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	mockConstructor := &MockStrategyConstructor{}
	mockRateLimiter := &MockRateLimiterForFactory{}
	
	config := map[string]interface{}{
		"bucket_size": 10,
		"key_prefix":  "test:",
	}

	mockConstructor.On("Name").Return("test_strategy")
	mockConstructor.On("NewFromConfig", config, mockRedis).Return(mockRateLimiter, nil)

	factory.RegisterStrategy(mockConstructor)

	rateLimiter, err := factory.CreateRateLimiter("test_strategy", config)

	assert.NoError(t, err)
	assert.NotNil(t, rateLimiter)
	
	// Should be wrapped with metrics decorator
	_, isDecorated := rateLimiter.(*MetricsDecorator)
	assert.True(t, isDecorated, "Rate limiter should be wrapped with metrics decorator")
	
	mockConstructor.AssertExpectations(t)
}

func TestFactory_CreateRateLimiter_UnsupportedStrategy(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	config := map[string]interface{}{
		"bucket_size": 10,
	}

	rateLimiter, err := factory.CreateRateLimiter("unsupported_strategy", config)

	assert.Error(t, err)
	assert.Nil(t, rateLimiter)
	assert.Contains(t, err.Error(), "unsupported rate limiter strategy")
}

func TestFactory_CreateRateLimiter_ConstructorError(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	mockConstructor := &MockStrategyConstructor{}
	
	config := map[string]interface{}{
		"invalid": "config",
	}

	mockConstructor.On("Name").Return("test_strategy")
	mockConstructor.On("NewFromConfig", config, mockRedis).Return((*MockRateLimiterForFactory)(nil), assert.AnError)

	factory.RegisterStrategy(mockConstructor)

	rateLimiter, err := factory.CreateRateLimiter("test_strategy", config)

	assert.Error(t, err)
	assert.Nil(t, rateLimiter)
	assert.Equal(t, assert.AnError, err)
	
	mockConstructor.AssertExpectations(t)
}

func TestFactory_WithMetrics(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)
	
	customMetrics := metrics.NewNoopCollector()
	factoryWithMetrics := factory.WithMetrics(customMetrics)

	assert.Equal(t, factory, factoryWithMetrics, "WithMetrics should return the same factory instance")
	assert.Equal(t, customMetrics, factory.metricsCollector)
}

func TestFactory_CreateRateLimiter_WithoutMetrics(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	// Set metrics collector to nil to test path without metrics
	factory.metricsCollector = nil

	mockConstructor := &MockStrategyConstructor{}
	mockRateLimiter := &MockRateLimiterForFactory{}
	
	config := map[string]interface{}{
		"bucket_size": 10,
		"key_prefix":  "test:",
	}

	mockConstructor.On("Name").Return("test_strategy")
	mockConstructor.On("NewFromConfig", config, mockRedis).Return(mockRateLimiter, nil)

	factory.RegisterStrategy(mockConstructor)

	rateLimiter, err := factory.CreateRateLimiter("test_strategy", config)

	assert.NoError(t, err)
	assert.NotNil(t, rateLimiter)
	
	// Should return the original rate limiter without decoration
	assert.Equal(t, mockRateLimiter, rateLimiter)
	
	mockConstructor.AssertExpectations(t)
}

func TestFactory_GetAvailableStrategies(t *testing.T) {
	mockRedis := &redis.Client{}
	factory := NewFactory(mockRedis)

	// Test with default strategies
	strategies := factory.GetAvailableStrategies()
	assert.Len(t, strategies, 3)
	assert.Contains(t, strategies, "token_bucket")
	assert.Contains(t, strategies, "sliding_window_log")
	assert.Contains(t, strategies, "sliding_window_counter")

	// Add custom strategy
	mockConstructor := &MockStrategyConstructor{}
	mockConstructor.On("Name").Return("custom_strategy")
	factory.RegisterStrategy(mockConstructor)

	strategies = factory.GetAvailableStrategies()
	assert.Len(t, strategies, 4)
	assert.Contains(t, strategies, "custom_strategy")
	
	mockConstructor.AssertExpectations(t)
}