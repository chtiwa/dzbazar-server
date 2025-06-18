package routes

import (
	"github.com/chtiwa/herbs-store-client/controllers"
	"github.com/chtiwa/herbs-store-client/middleware"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/orders")
	{
		orders.POST("", controllers.CreateOrder)
		orders.POST("/proxy", middleware.RequireAuthentication, controllers.CreateZrOrder)
		orders.GET("", middleware.RequireAuthentication, controllers.GetOrders)
		orders.POST("/excel", middleware.RequireAuthentication, controllers.ExportAsExcel)
		// based on the input data / status / user info (phone or name)
		orders.GET("/search", middleware.RequireAuthentication, controllers.GetOrdersBySearch)

		orders.GET("/:id", middleware.RequireAuthentication, controllers.GetOrder)
		orders.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.UpdateOrder)
		orders.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.DeleteOrder)
	}
}
