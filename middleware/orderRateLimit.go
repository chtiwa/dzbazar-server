package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
)

const (
	ipOrderWindow = time.Hour
	ipOrderMax    = 10
)

// OrderIPRateLimit silently drops order creation requests from IPs that have
// exceeded ipOrderMax submissions within the past hour.
func OrderIPRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:order:ip:%s", ip)

		count, err := initializers.RClient.Incr(initializers.Ctx, key).Result()
		if err != nil {
			// Redis failure is non-fatal — allow the request through.
			c.Next()
			return
		}

		if count == 1 {
			// First request in this window: set the expiry.
			initializers.RClient.Expire(initializers.Ctx, key, ipOrderWindow)
		}

		if count > ipOrderMax {
			// Silent drop: respond as if the order was accepted.
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Order received successfully",
			})
			return
		}

		c.Next()
	}
}
