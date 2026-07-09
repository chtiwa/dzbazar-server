package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ZrRoutes(router *gin.Engine) {
	g := router.Group("/v1/shops/:shopId/zr")
	g.Use(middleware.RequireAuthentication)
	{
		g.GET("/orders", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.GetZrOrders)
		g.POST("/orders", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.CreateZrOrder)
		g.POST("/orders/bulk", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.BulkCreateZrOrders)
		g.POST("/geo/refresh", middleware.RequireShopAccess("owner", "moderator"), controllers.RefreshZrGeo)
		g.GET("/geo/debug", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.DebugZrGeo)
	}
}
