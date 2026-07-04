package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OffersRoutes(router *gin.Engine) {
	admin := router.Group("/v1/shops/:shopId/offers",
		middleware.RequireAuthentication,
		middleware.RequireRoles("owner", "moderator"),
	)
	{
		admin.POST("", controllers.CreateOffer)
		admin.GET("", controllers.GetOffersByShop)
		admin.GET("/:id", controllers.GetOffer)
		admin.PATCH("/:id", controllers.UpdateOffer)
		admin.POST("/:id/publish", controllers.PublishOffer)
		admin.POST("/:id/archive", controllers.ArchiveOffer)
		admin.DELETE("/:id", controllers.DeleteOffer)
		admin.PUT("/:id/overrides/:landingPageId", controllers.UpsertOfferOverride)
		admin.DELETE("/:id/overrides/:landingPageId", controllers.DeleteOfferOverride)
	}

	store := router.Group("/v1/store/:slug")
	{
		store.POST("/offers/evaluate", controllers.EvaluateOffersPublic)
		store.POST("/offer-events", controllers.TrackOfferEventPublic)
	}
}
