package metrics

import "time"

// NoopCollector is a no-operation metrics collector for testing or when metrics are disabled
type NoopCollector struct{}

func NewNoopCollector() *NoopCollector {
	return &NoopCollector{}
}

func (n *NoopCollector) RecordRateLimitDecision(strategy string, allowed bool) {
	// No-op
}

func (n *NoopCollector) RecordRateLimitDuration(strategy string, duration time.Duration) {
	// No-op
}