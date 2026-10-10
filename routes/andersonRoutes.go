package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func AndersonRoutes(router *gin.Engine) {
	// Anderson and Navex are both Ecotrack: same handlers, carrier picked per group.
	for _, carrier := range []string{"anderson", "navex"} {
		g := router.Group("/v1/shops/:shopId/"+carrier, controllers.SetEcotrackCarrier(carrier))
		g.Use(middleware.RequireAuthentication)
		{
			g.GET("/orders", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.track"), controllers.GetAndersonOrders)
			g.POST("/orders", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ship"), controllers.CreateAndersonOrder)
			g.POST("/orders/bulk", middleware.RequireShopAccess(), middleware.RequireShopPermission("orders.ship"), controllers.BulkCreateAndersonOrders)
		}
	}
}
