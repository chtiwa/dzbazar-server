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
		// Staff and Logistics can view rates (needed to create orders), but only Owner can modify them
		rates.GET("", middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.GetDeliveryRates)
		rates.PATCH("", middleware.RequireRoles("Owner"), controllers.UpdateDeliveryRate)
		rates.PATCH("/bulk", middleware.RequireRoles("Owner"), controllers.BulkUpdateDeliveryRates)
	}

	// Public counterpart for the anonymous storefront checkout — no session exists there.
	public := router.Group("/v1/public/shops/:shopId/delivery-rates")
	{
		public.GET("", controllers.GetPublicDeliveryRates)
	}
}
