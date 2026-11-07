package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ShopsRoutes(router *gin.Engine) {
	shops := router.Group("/v1/shops")
	{
		shops.GET("", middleware.RequireAuthentication, controllers.GetMyShops)
		shops.POST("", middleware.RequireAuthentication, controllers.CreateShop)
		shops.GET("/invites", middleware.RequireAuthentication, controllers.ListMyInvites)
		shops.POST("/invites/:inviteId/accept", middleware.RequireAuthentication, controllers.AcceptInvite)
		shops.DELETE("/invites/:inviteId", middleware.RequireAuthentication, controllers.DeclineInvite)
		shops.GET("/by-slug/:slug", middleware.RequireAuthentication, controllers.IndexShopBySlug)
		shops.GET("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("settings.view"), controllers.GetShopByID)
		shops.PATCH("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("settings.edit"), controllers.UpdateShop)
		shops.DELETE("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.DeleteShop)
	}

	// Crawler link previews: edge rewrites bot UAs to /v1/og/<original path>.
	router.GET("/v1/og/*path", middleware.RateLimitByIP("og-preview", 300, time.Minute), controllers.OGPage)

	store := router.Group("/v1/store")
	{
		store.GET("/:slug", middleware.RateLimitByIP("store-lookup", 600, time.Minute), controllers.IndexShopBySlug)
	}
}
