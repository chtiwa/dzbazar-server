package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// need to use external library for cors
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		clientURL := os.Getenv("CLIENT_URL")
		clientURLv2 := os.Getenv("CLIENT_URL_V2")
		adminURL := os.Getenv("ADMIN_URL")
		superAdminURL := os.Getenv("SUPER_ADMIN_URL")

		allowed := origin != "" && (origin == clientURL || origin == adminURL || origin == clientURLv2 || origin == superAdminURL || origin == "http://localhost:5000")

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

		c.Next()
	}
}
