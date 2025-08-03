package metrics

import "time"

type Collector interface {
	RecordRateLimitDecision(strategy string, allowed bool)
	RecordRateLimitDuration(strategy string, duration time.Duration)
}
