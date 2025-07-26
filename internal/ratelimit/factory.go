package ratelimit

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Factory struct {
	redisClient *redis.Client
	strategies  map[string]StrategyConstructor
}

func NewFactory(redisClient *redis.Client) *Factory {
	f := &Factory{
		redisClient: redisClient,
		strategies:  make(map[string]StrategyConstructor),
	}

	f.RegisterStrategy(&TokenBucketConstructor{})
	f.RegisterStrategy(&SlidingWindowLogConstructor{})
	f.RegisterStrategy(&SlidingWindowCounterConstructor{})

	return f
}

func (f *Factory) RegisterStrategy(constructor StrategyConstructor) {
	f.strategies[constructor.Name()] = constructor
}

func (f *Factory) CreateRateLimiter(strategy string, config map[string]interface{}) (RateLimiter, error) {
	constructor, exists := f.strategies[strategy]
	if !exists {
		return nil, fmt.Errorf("unsupported rate limiter strategy: %s", strategy)
	}

	return constructor.NewFromConfig(config, f.redisClient)
}

func (f *Factory) GetAvailableStrategies() []string {
	strategies := make([]string, 0, len(f.strategies))
	for name := range f.strategies {
		strategies = append(strategies, name)
	}
	return strategies
}
