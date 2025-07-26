package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
)

type RateLimitConfig struct {
	KeyExtractor func(c *gin.Context) string
	OnLimitReached func(c *gin.Context, response ratelimit.RateLimitResponse)
	SkipSuccessfulRequests bool
}

func defaultKeyExtractor(c *gin.Context) string {
	clientID := c.GetHeader("X-Client-ID")
	if clientID == "" {
		clientID = c.ClientIP()
	}
	return clientID
}

func defaultOnLimitReached(c *gin.Context, response ratelimit.RateLimitResponse) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"message": "Too many requests",
	})
	c.Abort()
}

func RateLimit(rateLimiter ratelimit.RateLimiter, config ...*RateLimitConfig) gin.HandlerFunc {
	var cfg *RateLimitConfig
	if len(config) > 0 && config[0] != nil {
		cfg = config[0]
	} else {
		cfg = &RateLimitConfig{}
	}

	if cfg.KeyExtractor == nil {
		cfg.KeyExtractor = defaultKeyExtractor
	}
	if cfg.OnLimitReached == nil {
		cfg.OnLimitReached = defaultOnLimitReached
	}

	return func(c *gin.Context) {
		key := cfg.KeyExtractor(c)
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		response, err := rateLimiter.IsAllowed(ctx, key, time.Now())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limiter error",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		setRateLimitHeaders(c, response)

		if !response.Allowed {
			cfg.OnLimitReached(c, response)
			return
		}

		if !cfg.SkipSuccessfulRequests {
			c.Next()
		}
	}
}

func setRateLimitHeaders(c *gin.Context, response ratelimit.RateLimitResponse) {
	c.Header("RateLimit-Limit", strconv.FormatInt(response.Limit, 10))
	c.Header("RateLimit-Remaining", strconv.FormatInt(response.Remaining, 10))

	resetSeconds := int64(time.Until(response.ResetTime).Seconds())
	if resetSeconds < 0 {
		resetSeconds = 0
	}
	c.Header("RateLimit-Reset", strconv.FormatInt(resetSeconds, 10))

	if !response.Allowed && response.RetryAfter != nil {
		retryAfterSeconds := int64(response.RetryAfter.Seconds())
		if retryAfterSeconds < 0 {
			retryAfterSeconds = 0
		}
		c.Header("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	}
}