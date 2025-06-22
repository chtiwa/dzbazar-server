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
		clientURLv2 := os.Getenv("CLIENT_UR_V2")
		adminURL := os.Getenv("ADMIN_URL")

		if origin == clientURL || origin == adminURL || origin == clientURLv2 || origin == "http://localhost:5000" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, PATCH, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, token, key")
			// }

			// Handle OPTIONS method
			if c.Request.Method == "OPTIONS" {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}

			c.Next()
		}
	}
}
