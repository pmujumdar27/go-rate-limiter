package ratelimit

import (
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/pmujumdar27/go-rate-limiter/internal/config"
)

type StrategyManager interface {
	GetCurrentStrategy() (RateLimiter, error)

	UpdateStrategy(strategy string, config map[string]interface{}) error

	GetAvailableStrategies() []string
}

type ConfigBasedStrategyManager struct {
	config      *config.RateLimiterConfig
	redisClient *redis.Client
	factory     *Factory
}

func NewConfigBasedStrategyManager(cfg *config.RateLimiterConfig, redisClient *redis.Client) *ConfigBasedStrategyManager {
	return &ConfigBasedStrategyManager{
		config:      cfg,
		redisClient: redisClient,
		factory:     NewFactory(redisClient),
	}
}

func (m *ConfigBasedStrategyManager) GetCurrentStrategy() (RateLimiter, error) {
	strategy := m.config.Strategy

	var strategyConfig map[string]interface{}
	var err error

	constructor, exists := m.factory.strategies[strategy]
	if !exists {
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}

	switch strategy {
	case "token_bucket":
		strategyConfig, err = constructor.ConvertConfig(m.config.Strategies.TokenBucket)
	case "sliding_window_log":
		strategyConfig, err = constructor.ConvertConfig(m.config.Strategies.SlidingWindowLog)
	case "sliding_window_counter":
		strategyConfig, err = constructor.ConvertConfig(m.config.Strategies.SlidingWindowCounter)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to convert config for strategy %s: %w", strategy, err)
	}

	return m.factory.CreateRateLimiter(strategy, strategyConfig)
}

func (m *ConfigBasedStrategyManager) UpdateStrategy(strategy string, config map[string]interface{}) error {
	// TODO: Implement for admin API
	// This would involve:
	// 1. Validating the new strategy and config
	// 2. Updating the configuration (possibly persisting to file/database)
	// 3. Notifying the service to use the new strategy
	return fmt.Errorf("strategy updates not yet implemented - use configuration files")
}

func (m *ConfigBasedStrategyManager) GetAvailableStrategies() []string {
	return m.factory.GetAvailableStrategies()
}

