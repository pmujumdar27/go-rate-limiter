package ratelimit

const (
	// DefaultTTLBufferSeconds is the default buffer time in seconds added to TTL
	// to protect against clock drift and network latency
	DefaultTTLBufferSeconds = 60

	// MinimumTTLSeconds is the minimum TTL value for Redis keys to prevent
	// premature expiration
	MinimumTTLSeconds = 60

	// NanosecondsPerSecond is the conversion factor from nanoseconds to seconds
	NanosecondsPerSecond = 1e9
)
