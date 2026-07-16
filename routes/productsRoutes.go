package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ProductsRoutes(router *gin.Engine) {
	adminProducts := router.Group("/v1/shops/:shopId/products")
	{
		adminProducts.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetProductsByShopAdmin)
		adminProducts.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("products.create"), controllers.CreateProductByShop)
		adminProducts.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetProductByIDAdmin)
		adminProducts.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("products.edit"), controllers.UpdateProductByShop)
		adminProducts.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("products.delete"), controllers.DeleteProductByShop)
		adminProducts.PATCH("/:id/images", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("products.edit"), controllers.UpdateProductImagesByShop)
	}

	storeProducts := router.Group("/v1/store/:slug/products")
	storeProducts.Use(middleware.RateLimitByIP("store-products", 120, time.Minute))
	{
		storeProducts.GET("", controllers.GetActiveProductsBySlug)
		storeProducts.GET("/search", controllers.GetProductsBySearchBySlug)
		storeProducts.GET("/:id", controllers.IndexProductBySlug)
	}
}
