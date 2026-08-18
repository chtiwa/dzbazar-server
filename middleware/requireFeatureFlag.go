package middleware

import (
	"net/http"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
)

// RequireFeatureEnabled blocks a route while the given platform-wide feature
// flag is off — the kill switch a super-admin operator flips during an
// incident, no deploy needed. See services.IsFeatureEnabled for the cache
// behind it.
func RequireFeatureEnabled(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !services.IsFeatureEnabled(key) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "This feature is temporarily disabled by the platform. Please try again later.",
			})
			return
		}
		c.Next()
	}
}
