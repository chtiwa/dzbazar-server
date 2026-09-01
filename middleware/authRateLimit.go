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

// AllowShopAction is the per-shop fixed-window check behind RateLimitByShop,
// callable outside the middleware chain — for limits that depend on data the
// router doesn't have yet (e.g. a cap that only applies to free-tier shops,
// which needs the plan loaded first). Returns true when the action is
// allowed. Redis failure is non-fatal and allows the action, same as the
// middlewares.
func AllowShopAction(bucket string, shopID string, max int64, window time.Duration) bool {
	key := fmt.Sprintf("ratelimit:%s:shop:%s", bucket, shopID)
	count, err := initializers.RClient.Incr(initializers.Ctx, key).Result()
	if err != nil {
		return true
	}
	if count == 1 {
		initializers.RClient.Expire(initializers.Ctx, key, window)
	}
	return count <= max
}

// RateLimitByShop is RateLimitByIP keyed on :shopId instead of client IP —
// for authenticated per-shop actions where the IP isn't the right bucket
// (shared office NAT, mobile carrier IP rotation).
func RateLimitByShop(bucket string, max int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !AllowShopAction(bucket, c.Param("shopId"), max, window) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "Too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}
