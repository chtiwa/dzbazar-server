package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/orders")
	{
		orders.GET("", middleware.RequireAuthentication, controllers.GetOrdersByShopID)
		orders.POST("", controllers.CreateOrder)
		orders.POST("/proxy", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateZrOrder)
		orders.POST("/excel", middleware.RequireAuthentication, middleware.RequireRoles("Admin", "Moderator"), controllers.ExportAsExcel)
		// orders.GET("/search", middleware.RequireAuthentication, controllers.GetOrdersBySearch)
		// orders.GET("/filters", middleware.RequireAuthentication, middleware.RequireRoles("Admin", "User", "Moderator"), controllers.GetOrdersByStatus)

		orders.GET("/:id", middleware.RequireAuthentication, controllers.IndexOrderByShopID)
		// orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin", "User", "Moderator"), controllers.UpdateOrder)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteOrder)
	}
}
