package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func OsenRoutes(router *gin.Engine) {
	g := router.Group("/v1/shops/:shopId/osen")
	g.Use(middleware.RequireAuthentication)
	{
		g.GET("/orders", controllers.GetOsenOrders)
		g.POST("/orders", middleware.RequireRoles("owner", "moderator", "confirmation"), controllers.CreateOsenOrder)
		g.POST("/orders/bulk", middleware.RequireRoles("owner", "moderator", "confirmation"), controllers.BulkCreateOsenOrders)
	}
}
