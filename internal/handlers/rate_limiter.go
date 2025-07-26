package handlers

import (
	"context"
	"net/http"
	"strconv"
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

	rlh.setRateLimitHeaders(c, response)

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

func (rlh *RateLimitHandler) setRateLimitHeaders(c *gin.Context, response ratelimit.RateLimitResponse) {
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
