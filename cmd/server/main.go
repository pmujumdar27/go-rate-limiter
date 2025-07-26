package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/config"
	"github.com/pmujumdar27/go-rate-limiter/internal/handlers"
	"github.com/pmujumdar27/go-rate-limiter/internal/middleware"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	config          *config.Config
	redisClient     *redis.Client
	strategyManager ratelimit.StrategyManager
	router          *gin.Engine
	httpServer      *http.Server
}

func NewServer(cfg *config.Config) (*Server, error) {
	server := &Server{
		config: cfg,
	}

	if err := server.setupRedis(); err != nil {
		return nil, fmt.Errorf("failed to setup redis: %w", err)
	}

	if err := server.setupStrategyManager(); err != nil {
		return nil, fmt.Errorf("failed to setup strategy manager: %w", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return nil
}

func (s *Server) setupStrategyManager() error {
	s.strategyManager = ratelimit.NewConfigBasedStrategyManager(&s.config.RateLimiter, s.redisClient)
	return nil
}

func (s *Server) setupRoutes() {
	s.router = gin.Default()
	s.setupHandlers()
	s.setupHTTPServer()
}

func (s *Server) setupHandlers() {
	rateLimiter, err := s.strategyManager.GetCurrentStrategy()
	if err != nil {
		panic(fmt.Errorf("failed to get rate limiter from strategy manager: %w", err))
	}

	rateLimitHandler := handlers.NewRateLimitHandler(rateLimiter)
	demoHandler := handlers.NewDemoHandler()

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

	api := s.router.Group("/api")
	{
		api.GET("/unrestricted", demoHandler.UnrestrictedResource)
		api.GET("/restricted", middleware.RateLimit(rateLimiter), demoHandler.RestrictedResource)
	}
}

func (s *Server) setupHTTPServer() {
	s.httpServer = &http.Server{
		Addr:    s.config.Server.Port,
		Handler: s.router,
	}
}

func (s *Server) Run() error {
	go func() {
		log.Printf("Starting server on %s", s.config.Server.Port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
		return err
	}

	if err := s.redisClient.Close(); err != nil {
		log.Printf("Error closing Redis connection: %v", err)
	}

	log.Println("Server exited")
	return nil
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
