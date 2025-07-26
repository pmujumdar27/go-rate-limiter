package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RateLimitRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rate_limit_requests_total",
			Help: "Total number of rate limit requests by strategy and decision",
		},
		[]string{"strategy", "decision"},
	)

	RateLimitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rate_limit_duration_seconds",
			Help:    "Time spent processing rate limit requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"strategy"},
	)

	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operations_duration_seconds",
			Help:    "Time spent on Redis operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time spent processing HTTP requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	ActiveKeys = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rate_limit_active_keys",
			Help: "Number of active rate limit keys by strategy",
		},
		[]string{"strategy"},
	)

	RedisOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redis_operations_total",
			Help: "Total number of Redis operations by operation and status",
		},
		[]string{"operation", "status"},
	)
)
