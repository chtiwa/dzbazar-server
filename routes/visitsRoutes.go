package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func VisitsRoutes(router *gin.Engine) {
	// Public storefront beacon
	router.POST(
		"/v1/shops/:shopId/visits",
		middleware.RateLimitByIP("visits-record", 120, time.Minute),
		controllers.RecordVisit,
	)

	// Protected merchant read
	visits := router.Group("/v1/shops/:shopId/visits")
	visits.Use(middleware.RequireAuthentication)
	{
		visits.GET("", middleware.RequireShopAccess("owner", "moderator"), controllers.GetVisits)
	}
}
