package middleware

import (
	"net/http"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequirePlanFeature 403s admin routes when the shop's plan lacks the flag.
// Must run after RequireShopAccess-style shopId param is available.
func RequirePlanFeature(flag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		shopID, err := uuid.Parse(c.Param("shopId"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
			return
		}
		ok, err := services.HasPlanFeature(shopID, flag)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Your plan does not include this feature. Upgrade to unlock it.",
				"code":    "PLAN_FEATURE_LOCKED",
			})
			return
		}
		c.Next()
	}
}

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
