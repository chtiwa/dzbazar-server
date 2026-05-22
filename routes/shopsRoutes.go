package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ShopsRoutes(router *gin.Engine) {
	shops := router.Group("/api/v1/shops")
	{
		shops.GET("", middleware.RequireAuthentication, controllers.GetMyShops)
		shops.POST("", middleware.RequireAuthentication, controllers.CreateShop)
		shops.PATCH("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.UpdateShop)
		shops.DELETE("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.DeleteShop)
	}
}
