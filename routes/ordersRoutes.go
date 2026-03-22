package routes

import (
	"github.com/chtiwa/lk-parfumo-server/controllers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/orders")
	{
		orders.GET("", middleware.RequireAuthentication, controllers.GetOrders)
		orders.POST("", controllers.CreateOrder)
		orders.POST("/proxy", middleware.RequireAuthentication, controllers.CreateZrOrder)
		orders.POST("/excel", middleware.RequireAuthentication, controllers.ExportAsExcel)
		// based on the input data / status / user info (phone or name)
		orders.GET("/search", middleware.RequireAuthentication, controllers.GetOrdersBySearch)
		orders.GET("/filters", middleware.RequireAuthentication, middleware.RequireRoles("Admin", "User", "Moderator"), controllers.GetOrdersByStatus)

		orders.GET("/:id", middleware.RequireAuthentication, controllers.GetOrder)
		orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.UpdateOrder)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.DeleteOrder)
	}
}
