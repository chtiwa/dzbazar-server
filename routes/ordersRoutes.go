package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/v1/shops/:shopId/orders")
	{
		orders.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.GetOrdersByShopID)
		orders.POST("", middleware.OrderIPRateLimit(), controllers.CreateOrderByShopID)
		orders.POST("/excel", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.ExportAsExcel)

		orders.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.IndexOrderByShopID)
		orders.GET("/:id/status-history", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.GetOrderStatusHistory)
		orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.UpdateOrderByShopID)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.DeleteOrderByShopID)
		orders.POST("/:id/ban-client", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.BanOrderClient)
	}
}
