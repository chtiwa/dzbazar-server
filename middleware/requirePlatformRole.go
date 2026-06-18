package middleware

import (
	"net/http"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

// RequirePlatformRole gates a route by the user's platform-wide role
// (User.PlatformRole), which is orthogonal to the tenant roles checked by
// RequireRoles/RequireShopAccess. Must run after RequireAuthentication.
func RequirePlatformRole(allowed ...string) gin.HandlerFunc {
	allowedMap := map[string]bool{}
	for _, a := range allowed {
		allowedMap[a] = true
	}

	return func(c *gin.Context) {
		userIf, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
			})
			return
		}

		user, ok := userIf.(models.User)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Internal server error: invalid user format",
			})
			return
		}

		if user.PlatformRole == "" || !allowedMap[user.PlatformRole] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "You do not have permission to access this resource",
			})
			return
		}

		c.Next()
	}
}

// RequireSuperAdmin is sugar for RequirePlatformRole("super_admin").
func RequireSuperAdmin() gin.HandlerFunc {
	return RequirePlatformRole("super_admin")
}
