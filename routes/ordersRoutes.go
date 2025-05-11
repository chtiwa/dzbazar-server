package routes

import (
	"github.com/chtiwa/herbs-store-client/controllers"
	"github.com/gin-gonic/gin"
)

func OrdersRoutes(router *gin.Engine) {
	orders := router.Group("/orders")
	{
		orders.POST("", controllers.CreateOrder)
		// based on the input data / status / user info (phone or name)
		orders.GET("", controllers.GetOrders)

		orders.GET("/:id", controllers.GetOrder)
		orders.PATCH("/:id", controllers.UpdateOrder)
		orders.DELETE("/:id", controllers.DeleteOrder)
	}
}
