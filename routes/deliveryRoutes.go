package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DeliveryRatesRoutes(router *gin.Engine) {
	rates := router.Group("/v1/shops/:shopId/delivery-rates")
	rates.Use(middleware.RequireAuthentication)
	{
		// All roles can view rates (needed to create orders)
		rates.GET("", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.GetDeliveryRates)
		// Only owner and moderator can modify rates
		rates.PATCH("", middleware.RequireShopAccess("owner", "moderator"), controllers.UpdateDeliveryRate)
		rates.PATCH("/bulk", middleware.RequireShopAccess("owner", "moderator"), controllers.BulkUpdateDeliveryRates)
	}

	// Public counterpart for the anonymous storefront checkout — no session exists there.
	public := router.Group("/v1/public/shops/:shopId/delivery-rates")
	{
		public.GET("", controllers.GetPublicDeliveryRates)
	}
}
