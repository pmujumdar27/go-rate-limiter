package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

func Load() (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if err := loadConfigFile(v); err != nil {
		return nil, err
	}

	if err := loadDotEnvFile(v); err != nil {
		return nil, err
	}

	loadEnvironmentVariables(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", ":8080")
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.password", "")

	v.SetDefault("rate_limiter.strategy", "sliding_window_counter")

	v.SetDefault("rate_limiter.strategies.token_bucket.key_prefix", "rl:tb:")
	v.SetDefault("rate_limiter.strategies.token_bucket.ttl_buffer_seconds", 5)
	v.SetDefault("rate_limiter.strategies.token_bucket.bucket_size", 100)
	v.SetDefault("rate_limiter.strategies.token_bucket.refill_rate_per_second", 10)

	v.SetDefault("rate_limiter.strategies.sliding_window_log.key_prefix", "rl:swl:")
	v.SetDefault("rate_limiter.strategies.sliding_window_log.ttl_buffer_seconds", 30)
	v.SetDefault("rate_limiter.strategies.sliding_window_log.window_size_seconds", 3600)
	v.SetDefault("rate_limiter.strategies.sliding_window_log.bucket_size", 1000)

	v.SetDefault("rate_limiter.strategies.sliding_window_counter.key_prefix", "rl:swc:")
	v.SetDefault("rate_limiter.strategies.sliding_window_counter.ttl_buffer_seconds", 15)
	v.SetDefault("rate_limiter.strategies.sliding_window_counter.window_size_seconds", 3600)
	v.SetDefault("rate_limiter.strategies.sliding_window_counter.bucket_size", 1000)
}

func loadConfigFile(v *viper.Viper) error {
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}
	return nil
}

func loadDotEnvFile(v *viper.Viper) error {
	envFile := ".env"
	if _, err := os.Stat(envFile); err == nil {
		v.SetConfigFile(envFile)
		v.SetConfigType("env")
		if err := v.MergeInConfig(); err != nil {
			return fmt.Errorf("failed to read .env file: %w", err)
		}
	}
	return nil
}

func loadEnvironmentVariables(v *viper.Viper) {
	v.SetEnvPrefix("GO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	for _, key := range []string{
		"SERVER_PORT",
		"REDIS_HOST",
		"REDIS_PORT",
		"REDIS_PASSWORD",
		"REDIS_DB",
	} {
		if val := os.Getenv("GO_" + key); val != "" {
			v.Set(strings.ToLower(strings.ReplaceAll(key, "_", ".")), val)
		}
	}
}
