package ratelimit

import (
	"context"
	"time"

	"github.com/pmujumdar27/go-rate-limiter/internal/metrics"
)

type MetricsDecorator struct {
	rateLimiter RateLimiter
	collector   metrics.Collector
	strategy    string
}

func NewMetricsDecorator(rateLimiter RateLimiter, collector metrics.Collector, strategy string) *MetricsDecorator {
	return &MetricsDecorator{
		rateLimiter: rateLimiter,
		collector:   collector,
		strategy:    strategy,
	}
}

func (m *MetricsDecorator) IsAllowed(ctx context.Context, key string, timestamp time.Time) (RateLimitResponse, error) {
	start := time.Now()

	response, err := m.rateLimiter.IsAllowed(ctx, key, timestamp)

	duration := time.Since(start)
	m.collector.RecordRateLimitDuration(m.strategy, duration)

	if err == nil {
		m.collector.RecordRateLimitDecision(m.strategy, response.Allowed)
	}

	return response, err
}

func (m *MetricsDecorator) Reset(ctx context.Context, key string) error {
	return m.rateLimiter.Reset(ctx, key)
}
