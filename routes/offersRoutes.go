package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OffersRoutes(router *gin.Engine) {
	admin := router.Group("/v1/shops/:shopId/offers", middleware.RequireAuthentication)
	{
		admin.POST("", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.create"), controllers.CreateOffer)
		admin.GET("", middleware.RequireShopAccess("owner", "moderator"), controllers.GetOffersByShop)
		admin.GET("/:id", middleware.RequireShopAccess("owner", "moderator"), controllers.GetOffer)
		admin.PATCH("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.edit"), controllers.UpdateOffer)
		admin.POST("/:id/publish", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.archive"), controllers.PublishOffer)
		admin.POST("/:id/archive", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.archive"), controllers.ArchiveOffer)
		admin.DELETE("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.delete"), controllers.DeleteOffer)
		admin.PUT("/:id/overrides/:landingPageId", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.edit"), controllers.UpsertOfferOverride)
		admin.DELETE("/:id/overrides/:landingPageId", middleware.RequireShopAccess(), middleware.RequireShopPermission("offers.edit"), controllers.DeleteOfferOverride)
	}

	store := router.Group("/v1/store/:slug")
	store.Use(middleware.RateLimitByIP("store-offers", 120, time.Minute))
	{
		store.POST("/offers/evaluate", controllers.EvaluateOffersPublic)
		store.POST("/offer-events", controllers.TrackOfferEventPublic)
	}
}
