package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func LeopardRoutes(router *gin.Engine) {
	g := router.Group("/v1/shops/:shopId/leopard")
	g.Use(middleware.RequireAuthentication)
	{
		g.POST("/orders", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ship"), controllers.CreateLeopardOrder)
	}
}
