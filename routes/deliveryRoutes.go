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
		// Staff can view rates, but only Owner (or Admin) can modify them
		rates.GET("", middleware.RequireRoles("Owner", "Staff"), controllers.GetDeliveryRates)
		rates.PATCH("", middleware.RequireRoles("Owner"), controllers.UpdateDeliveryRate)
		rates.PATCH("/bulk", middleware.RequireRoles("Owner"), controllers.BulkUpdateDeliveryRates)
	}
}
