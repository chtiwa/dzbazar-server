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
		g.GET("/orders", controllers.GetZrOrders)
		g.POST("/orders", middleware.RequireRoles("Owner", "Logistics"), controllers.CreateZrOrder)
		g.POST("/orders/bulk", middleware.RequireRoles("Owner", "Logistics"), controllers.BulkCreateZrOrders)
		g.POST("/geo/refresh", controllers.RefreshZrGeo)
		g.GET("/geo/debug", controllers.DebugZrGeo)
	}
}
