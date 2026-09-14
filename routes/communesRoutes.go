package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

// CommunesRoutes is public/no-auth — same reference-data pattern as
// public wilayas, consumed by the anonymous storefront checkout form.
func CommunesRoutes(router *gin.Engine) {
	communes := router.Group("/v1/communes")
	communes.Use(middleware.RateLimitByIP("communes", 120, time.Minute))
	{
		communes.GET("", controllers.ListCommunes)
	}
}
