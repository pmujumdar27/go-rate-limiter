package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/config"
	"github.com/pmujumdar27/go-rate-limiter/internal/handlers"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
	"github.com/redis/go-redis/v9"
)

var (
	cfg         *config.Config
	redisClient *redis.Client
	rateLimiter ratelimit.RateLimiter
)

// TODO: Make this logic cleaner, and later maybe add an admin API to change the rate limiter
func initRateLimiter(r *redis.Client) {
	config := map[string]interface{}{
		"window_size": 10 * time.Second,
		"bucket_size": int64(10),
	}

	var err error
	rateLimiter, err = ratelimit.NewRateLimiter(ratelimit.SlidingWindowCounterStrategy, r, "rate_limit:swc", config)
	if err != nil {
		panic(err)
	}

	// rateLimiter = ratelimit.NewTokenBucketRateLimiter(10, 2, r, "rate_limit:tb")
	// rateLimiter = ratelimit.NewSlidingWindowLogRateLimiter(10*time.Second, r, "rate_limit:swl", 10)
	// rateLimiter = ratelimit.NewSlidingWindowCounterRateLimiter(10*time.Second, r, "rate_limit:swc", 10)
}

func initRedisClient() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

func main() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		panic(fmt.Errorf("failed to load config: %w", err))
	}

	initRedisClient()
	initRateLimiter(redisClient)

	rateLimitHandler := handlers.NewRateLimitHandler(rateLimiter)

	r := gin.Default()

	r.GET("/health", handlers.Health)

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "go-rate-limiter",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	r.POST("/rate-limit", rateLimitHandler.RateLimit)
	r.POST("/rate-limit/reset", rateLimitHandler.ResetRateLimit)

	r.Run(cfg.Server.Port)
}
