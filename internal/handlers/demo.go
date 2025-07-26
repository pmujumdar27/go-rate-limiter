package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type DemoHandler struct{}

func NewDemoHandler() *DemoHandler {
	return &DemoHandler{}
}

func (d *DemoHandler) UnrestrictedResource(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message":   "Access granted to unrestricted resource",
		"timestamp": time.Now().UTC(),
		"path":      c.Request.URL.Path,
		"client_ip": c.ClientIP(),
		"user_agent": c.GetHeader("User-Agent"),
		"data": gin.H{
			"resource_id": "unrestricted-001",
			"content":     "This resource has no rate limiting applied",
			"access_count": "unlimited",
		},
	})
}

func (d *DemoHandler) RestrictedResource(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message":   "Access granted to restricted resource",
		"timestamp": time.Now().UTC(),
		"path":      c.Request.URL.Path,
		"client_ip": c.ClientIP(),
		"user_agent": c.GetHeader("User-Agent"),
		"data": gin.H{
			"resource_id": "restricted-001", 
			"content":     "This resource is protected by rate limiting",
			"access_count": "limited by rate limiter",
		},
	})
}