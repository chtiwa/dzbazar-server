package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireRoles(allowed ...string) gin.HandlerFunc {
	allowedMap := map[string]bool{}
	for _, a := range allowed {
		allowedMap[a] = true
	}

	return func(c *gin.Context) {
		roleIf, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "no role in context"})
			return
		}

		role := roleIf.(string)
		if !allowedMap[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}
