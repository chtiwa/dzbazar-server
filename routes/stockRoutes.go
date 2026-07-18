package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func StockRoutes(router *gin.Engine) {
	stock := router.Group("/v1/shops/:shopId/stock")
	{
		stock.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetShopStock)
		stock.PATCH("/:combinationId", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("products.edit"), controllers.UpdateStockQuantity)
	}
}
