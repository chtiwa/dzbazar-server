package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RequireRoles(allowed ...string) gin.HandlerFunc {
	// Pre-compute the map for O(1) lookups
	allowedMap := map[string]bool{}
	for _, a := range allowed {
		allowedMap[a] = true
	}

	return func(c *gin.Context) {
		roleIf, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Authentication required or role missing",
			})
			return
		}

		// SAFELY assert the type to prevent panics
		role, ok := roleIf.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Internal server error: invalid role format",
			})
			return
		}

		// Check if the role is in our allowed map
		if !allowedMap[role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You do not have permission to perform this action",
			})
			return
		}

		c.Next()
	}
}
