package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
)

// RateLimitByIP rejects an endpoint with 429 once a client IP exceeds max
// requests within window. Used on auth endpoints (login, OTP verify,
// password reset) to blunt brute-force/credential-stuffing attempts.
func RateLimitByIP(bucket string, max int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:%s:ip:%s", bucket, ip)

		count, err := initializers.RClient.Incr(initializers.Ctx, key).Result()
		if err != nil {
			// Redis failure is non-fatal — allow the request through.
			c.Next()
			return
		}

		if count == 1 {
			initializers.RClient.Expire(initializers.Ctx, key, window)
		}

		if count > max {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many attempts, please try again later",
			})
			return
		}

		c.Next()
	}
}
