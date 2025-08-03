package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type PrometheusCollector struct {
	rateLimitDecisions *prometheus.CounterVec
	rateLimitDuration  *prometheus.HistogramVec
}

func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{
		rateLimitDecisions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limit_requests_total",
				Help: "Total number of rate limit decisions by strategy and outcome",
			},
			[]string{"strategy", "decision"},
		),
		rateLimitDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "rate_limit_duration_seconds",
				Help: "Time taken to process rate limit checks",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"strategy"},
		),
	}
}

func (p *PrometheusCollector) RecordRateLimitDecision(strategy string, allowed bool) {
	decision := "denied"
	if allowed {
		decision = "allowed"
	}
	p.rateLimitDecisions.WithLabelValues(strategy, decision).Inc()
}

func (p *PrometheusCollector) RecordRateLimitDuration(strategy string, duration time.Duration) {
	p.rateLimitDuration.WithLabelValues(strategy).Observe(duration.Seconds())
}