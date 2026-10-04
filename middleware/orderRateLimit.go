package middleware

import (
	"fmt"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
)

// CtxIPRateLimited marks a request over the per-IP cap; the order controller stores it hidden instead of dropping it.
const CtxIPRateLimited = "ipRateLimited"

const (
	ipOrderWindow = time.Hour
	ipOrderMax    = 30
)

// OrderIPRateLimit flags (never drops) order requests from IPs that exceeded
// ipOrderMax submissions per shop within the past hour.
func OrderIPRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsStaffOrder(c) {
			c.Next()
			return
		}

		ip := c.ClientIP()
		key := fmt.Sprintf("ratelimit:order:ip:%s:%s", c.Param("shopId"), ip)

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
			c.Set(CtxIPRateLimited, true)
		}

		c.Next()
	}
}
