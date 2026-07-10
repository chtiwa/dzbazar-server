package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func LandingPagesRoutes(router *gin.Engine) {
	adminShop := router.Group("/v1/shops")
	{
		adminLandingPages := adminShop.Group("/:shopId/landing-pages")
		{
			adminLandingPages.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.CreateLandingPageByShop)
			adminLandingPages.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetLandingPagesByShop)
			adminLandingPages.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetLandingPageByShop)

			adminLandingPages.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.UpdateLandingPageByShop)
			adminLandingPages.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.DeleteLandingPageByShop)
		}
	}

	publicLandingPages := adminShop.Group("/landing-pages")
	{
		publicLandingPages.GET("/:id", middleware.RateLimitByIP("landing-page-public", 60, time.Minute), controllers.IndexLandingPage)
	}
}
