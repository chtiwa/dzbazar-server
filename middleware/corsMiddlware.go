package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// IsAllowedOrigin is the single source of truth for which browser origins
// may talk to this API — shared by CORSMiddleware (HTTP) and the websocket
// upgrader's CheckOrigin (realtime package), so the two can't drift apart.
func IsAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	clientURL := os.Getenv("CLIENT_URL")
	clientURLv2 := os.Getenv("CLIENT_URL_V2")
	adminURL := os.Getenv("ADMIN_URL")
	superAdminURL := os.Getenv("SUPER_ADMIN_URL")

	if origin == clientURL || origin == adminURL || origin == clientURLv2 || origin == superAdminURL {
		return true
	}

	// Dev-only convenience origin — never allowed in production.
	if os.Getenv("APP_ENV") != "production" && origin == "http://localhost:5000" {
		return true
	}

	return false
}

// need to use external library for cors
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowed := IsAllowedOrigin(origin)

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, token, key, X-Shop-ID")
		}

		// Handle OPTIONS preflight explicitly either way — never fall through
		// without a response, which previously hung the request when the
		// origin didn't match (or was absent, e.g. non-browser clients).
		if c.Request.Method == "OPTIONS" {
			if !allowed {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		// CSRF guard: CORS headers alone don't stop "simple" cross-site POSTs
		// (text/plain, no preflight) from running with the victim's SameSite=None
		// cookies. Browsers always send Origin on cross-site unsafe requests, so
		// reject any that isn't ours. No Origin (curl, server-to-server) passes.
		if origin != "" && !allowed {
			switch c.Request.Method {
			case http.MethodGet, http.MethodHead:
			default:
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "origin not allowed"})
				return
			}
		}

		c.Next()
	}
}
