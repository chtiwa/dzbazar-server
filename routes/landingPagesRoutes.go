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
			adminLandingPages.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.create"), controllers.CreateLandingPageByShop)
			adminLandingPages.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetLandingPagesByShop)
			adminLandingPages.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetLandingPageByShop)

			adminLandingPages.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.edit"), controllers.UpdateLandingPageByShop)
			adminLandingPages.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("landing_pages.delete"), controllers.DeleteLandingPageByShop)
			adminLandingPages.GET("/generated-images", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.ListGeneratedImages)
			adminLandingPages.GET("/image-campaigns/:campaignId", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetCampaignImages)
			adminLandingPages.POST("/image-campaigns/generate-image", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), middleware.RateLimitByShop("landing-page-image-set", 20, time.Minute), controllers.GenerateCampaignImage)
			adminLandingPages.DELETE("/image-campaigns/:campaignId/images/:index", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.DeleteCampaignImage)
		}
	}

	publicLandingPages := adminShop.Group("/landing-pages")
	{
		publicLandingPages.GET("/:id", middleware.RateLimitByIP("landing-page-public", 60, time.Minute), controllers.IndexLandingPage)
	}
}
