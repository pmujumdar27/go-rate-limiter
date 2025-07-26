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

type Server struct {
	config      *config.Config
	redisClient *redis.Client
	rateLimiter ratelimit.RateLimiter
	router      *gin.Engine
}

func NewServer(cfg *config.Config) (*Server, error) {
	server := &Server{
		config: cfg,
	}

	if err := server.setupRedis(); err != nil {
		return nil, fmt.Errorf("failed to setup redis: %w", err)
	}

	if err := server.setupRateLimiter(); err != nil {
		return nil, fmt.Errorf("failed to setup rate limiter: %w", err)
	}

	server.setupRoutes()
	return server, nil
}

func (s *Server) setupRedis() error {
	s.redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", s.config.Redis.Host, s.config.Redis.Port),
		Password: s.config.Redis.Password,
		DB:       s.config.Redis.DB,
	})
	return nil
}

// TODO: Make this logic cleaner, and later maybe add an admin API to change the rate limiter
func (s *Server) setupRateLimiter() error {
	config := map[string]interface{}{
		"window_size": 10 * time.Second,
		"bucket_size": int64(10),
	}

	var err error
	s.rateLimiter, err = ratelimit.NewRateLimiter(ratelimit.SlidingWindowCounterStrategy, s.redisClient, "rate_limit:swc", config)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) setupRoutes() {
	s.router = gin.Default()
	rateLimitHandler := handlers.NewRateLimitHandler(s.rateLimiter)

	s.router.GET("/health", handlers.Health)

	s.router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "go-rate-limiter",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	s.router.POST("/rate-limit", rateLimitHandler.RateLimit)
	s.router.POST("/rate-limit/reset", rateLimitHandler.ResetRateLimit)
}

func (s *Server) Run() error {
	return s.router.Run(s.config.Server.Port)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Errorf("failed to load config: %w", err))
	}

	server, err := NewServer(cfg)
	if err != nil {
		panic(fmt.Errorf("failed to create server: %w", err))
	}

	if err := server.Run(); err != nil {
		panic(fmt.Errorf("failed to run server: %w", err))
	}
}
