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

// TooManyFailedLogins reports whether an email has hit the failed-login cap
// within window — checked inside Login (after body bind, before password
// compare) since the email isn't known at route-middleware time.
func TooManyFailedLogins(email string, max int64, window time.Duration) bool {
	key := fmt.Sprintf("ratelimit:login:email:%s", email)
	count, err := initializers.RClient.Get(initializers.Ctx, key).Int64()
	if err != nil {
		return false
	}
	return count >= max
}

// RecordFailedLogin increments the per-email failed-login counter, starting
// the window on the first failure.
func RecordFailedLogin(email string, window time.Duration) {
	key := fmt.Sprintf("ratelimit:login:email:%s", email)
	count, err := initializers.RClient.Incr(initializers.Ctx, key).Result()
	if err != nil {
		return
	}
	if count == 1 {
		initializers.RClient.Expire(initializers.Ctx, key, window)
	}
}

// ClearFailedLogins resets the per-email failed-login counter on success.
func ClearFailedLogins(email string) {
	key := fmt.Sprintf("ratelimit:login:email:%s", email)
	initializers.RClient.Del(initializers.Ctx, key)
}

// TooManyOTPAttempts and RecordOTPAttempt guard OTP verification (VerifyUser,
// ResetPassword) against unlimited guessing of a 6-digit code — same
// fixed-window pattern as the failed-login counters above but under a
// distinct key prefix ("otp", not "login") so the two can't be sidestepped
// against each other. bucket distinguishes verify-otp from reset-password so
// one doesn't burn the other's attempt budget.
func TooManyOTPAttempts(bucket, email string, max int64) bool {
	key := fmt.Sprintf("ratelimit:otp:%s:email:%s", bucket, email)
	count, err := initializers.RClient.Get(initializers.Ctx, key).Int64()
	if err != nil {
		return false
	}
	return count >= max
}

// RecordOTPAttempt increments the per-email OTP attempt counter, starting a
// window equal to the OTP's own lifetime on the first attempt so the counter
// never outlives the code it's guarding.
func RecordOTPAttempt(bucket, email string, window time.Duration) {
	key := fmt.Sprintf("ratelimit:otp:%s:email:%s", bucket, email)
	count, err := initializers.RClient.Incr(initializers.Ctx, key).Result()
	if err != nil {
		return
	}
	if count == 1 {
		initializers.RClient.Expire(initializers.Ctx, key, window)
	}
}

// ClearOTPAttempts resets the per-email OTP attempt counter after a
// successful verification or when a fresh OTP is issued.
func ClearOTPAttempts(bucket, email string) {
	key := fmt.Sprintf("ratelimit:otp:%s:email:%s", bucket, email)
	initializers.RClient.Del(initializers.Ctx, key)
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
