package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pmujumdar27/go-rate-limiter/internal/ratelimit"
)

type RateLimitHandler struct {
	rateLimiter ratelimit.RateLimiter
}

func NewRateLimitHandler(rateLimiter ratelimit.RateLimiter) *RateLimitHandler {
	return &RateLimitHandler{
		rateLimiter: rateLimiter,
	}
}

func (rlh *RateLimitHandler) RateLimit(c *gin.Context) {
	clientID := c.GetHeader("X-Client-ID")
	if clientID == "" {
		clientID = c.ClientIP()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := rlh.rateLimiter.IsAllowed(ctx, clientID, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Rate limiter error",
			"message": err.Error(),
		})
		return
	}

	if !response.Allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"allowed":  false,
			"metadata": response.Metadata,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"allowed":  true,
		"metadata": response.Metadata,
	})
}

func (rlh *RateLimitHandler) ResetRateLimit(c *gin.Context) {
	clientID := c.GetHeader("X-Client-ID")
	if clientID == "" {
		clientID = c.ClientIP()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := rlh.rateLimiter.Reset(ctx, clientID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Reset error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Rate limit reset successfully",
		"client_id": clientID,
	})
}
