package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ProductsRoutes(router *gin.Engine) {
	adminProducts := router.Group("/v1/shops/:shopId/products")
	{
		adminProducts.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.GetProductsByShopAdmin)
		adminProducts.POST("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.CreateProductByShop)
		adminProducts.GET("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.GetProductByIDAdmin)
		adminProducts.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.UpdateProductByShop)
		adminProducts.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.DeleteProductByShop)
		adminProducts.PATCH("/:id/images", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.UpdateProductImagesByShop)
	}

	storeProducts := router.Group("/v1/store/:slug/products")
	{
		storeProducts.GET("", controllers.GetActiveProductsBySlug)
		storeProducts.GET("/search", controllers.GetProductsBySearchBySlug)
		storeProducts.GET("/:id", controllers.IndexProductBySlug)
	}
}
