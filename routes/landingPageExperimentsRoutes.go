package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func LandingPageExperimentsRoutes(router *gin.Engine) {
	adminShop := router.Group("/v1/shops")
	{
		adminExperiments := adminShop.Group("/:shopId/landing-page-experiments")
		{
			adminExperiments.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.create"), controllers.CreateExperimentByShop)
			adminExperiments.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetExperimentsByShop)
			adminExperiments.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetExperimentByShop)
			adminExperiments.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.edit"), controllers.UpdateExperimentByShop)
			adminExperiments.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.delete"), controllers.DeleteExperimentByShop)
		}
	}

	publicExperiments := adminShop.Group("/landing-page-experiments")
	{
		publicExperiments.POST("/:id/assign", middleware.RateLimitByIP("landing-page-experiment-assign", 60, time.Minute), controllers.AssignExperimentVariantPublic)
	}
}
