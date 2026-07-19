package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/v1/shops/:shopId/orders")
	{
		orders.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.view"), controllers.GetOrdersByShopID)
		orders.POST("", middleware.DetectStaffOrder, middleware.OrderIPRateLimit(), controllers.CreateOrderByShopID)
		orders.POST("/excel", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.export"), controllers.ExportAsExcel)

		orders.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.view"), controllers.IndexOrderByShopID)
		orders.GET("/:id/status-history", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.status_history"), controllers.GetOrderStatusHistory)
		orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.edit"), controllers.UpdateOrderByShopID)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.delete"), controllers.DeleteOrderByShopID)
		orders.POST("/:id/ban-client", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ban_client"), controllers.BanOrderClient)
	}
}
