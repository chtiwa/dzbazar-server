package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/api/v1/shops/:shopId/orders")
	{
		orders.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.GetOrdersByShopID)
		orders.POST("", controllers.CreateOrderByShopID)
		orders.POST("/proxy", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.CreateZrOrder)
		orders.POST("/excel", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.ExportAsExcel)

		orders.GET("/:id", middleware.RequireAuthentication, controllers.IndexOrderByShopID)
		orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.UpdateOrderByShopID)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.DeleteOrderByShopID)
	}
}
