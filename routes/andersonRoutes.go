package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func AndersonRoutes(router *gin.Engine) {
	g := router.Group("/v1/shops/:shopId/anderson")
	g.Use(middleware.RequireAuthentication)
	{
		g.POST("/orders", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ship"), controllers.CreateAndersonOrder)
		g.POST("/orders/bulk", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ship"), controllers.BulkCreateAndersonOrders)
	}
}
