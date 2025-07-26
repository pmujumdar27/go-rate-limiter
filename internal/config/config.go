package config

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Redis       RedisConfig       `mapstructure:"redis"`
	RateLimiter RateLimiterConfig `mapstructure:"rate_limiter"`
}

type ServerConfig struct {
	Port string `mapstructure:"port"`
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type RateLimiterConfig struct {
	Strategy   string                        `mapstructure:"strategy"`
	Strategies RateLimiterStrategiesConfig   `mapstructure:"strategies"`
}

type RateLimiterStrategiesConfig struct {
	TokenBucket         TokenBucketConfig         `mapstructure:"token_bucket"`
	SlidingWindowLog    SlidingWindowLogConfig    `mapstructure:"sliding_window_log"`
	SlidingWindowCounter SlidingWindowCounterConfig `mapstructure:"sliding_window_counter"`
}

type TokenBucketConfig struct {
	KeyPrefix           string `mapstructure:"key_prefix"`
	TTLBufferSeconds    int    `mapstructure:"ttl_buffer_seconds"`
	BucketSize          int64  `mapstructure:"bucket_size"`
	RefillRatePerSecond int64  `mapstructure:"refill_rate_per_second"`
}

type SlidingWindowLogConfig struct {
	KeyPrefix         string `mapstructure:"key_prefix"`
	TTLBufferSeconds  int    `mapstructure:"ttl_buffer_seconds"`
	WindowSizeSeconds int    `mapstructure:"window_size_seconds"`
	BucketSize        int64  `mapstructure:"bucket_size"`
}

type SlidingWindowCounterConfig struct {
	KeyPrefix         string `mapstructure:"key_prefix"`
	TTLBufferSeconds  int    `mapstructure:"ttl_buffer_seconds"`
	WindowSizeSeconds int    `mapstructure:"window_size_seconds"`
	BucketSize        int64  `mapstructure:"bucket_size"`
}
