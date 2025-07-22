package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/handlers"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
	"github.com/redis/go-redis/v9"
)

var (
	redisClient *redis.Client
	rateLimiter ratelimit.RateLimiter
)

func initRateLimiter(r *redis.Client) {
	rateLimiter = ratelimit.NewTokenBucketRateLimiter(10, 2, r, "rate_limit:tb")
	// rateLimiter = ratelimit.NewSlidingWindowLogRateLimiter(10*time.Second, r, "rate_limit:swl", 10)
}

func initRedisClient() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
}

func main() {
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

	// Rate limiting endpoints
	r.POST("/rate-limit", rateLimitHandler.RateLimit)
	r.POST("/rate-limit/reset", rateLimitHandler.ResetRateLimit)

	r.Run(":8080")
}
