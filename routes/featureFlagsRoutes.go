package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

// FeatureFlagsRoutes exposes the read-only, public flag-state endpoint.
// Writes/toggles stay under /v1/super-admin/feature-flags (superAdminRoutes.go).
func FeatureFlagsRoutes(router *gin.Engine) {
	router.GET("/v1/feature-flags/:key", middleware.RateLimitByIP("feature-flag-read", 120, time.Minute), controllers.GetFeatureFlagPublic)
}
