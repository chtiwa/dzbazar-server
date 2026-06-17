package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ShopsRoutes(router *gin.Engine) {
	shops := router.Group("/v1/shops")
	{
		shops.GET("", middleware.RequireAuthentication, controllers.GetMyShops)
		shops.POST("", middleware.RequireAuthentication, controllers.CreateShop)
		shops.GET("/by-slug/:slug", middleware.RequireAuthentication, controllers.IndexShopBySlug)
		shops.GET("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.GetShopByID)
		shops.PATCH("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.UpdateShop)
		shops.DELETE("/:shopId", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.DeleteShop)
	}

	store := router.Group("/v1/store")
	{
		store.GET("/:slug", controllers.IndexShopBySlug)
	}
}
