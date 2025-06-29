package routes

import (
	"github.com/chtiwa/herbs-store-client/controllers"
	"github.com/chtiwa/herbs-store-client/middleware"
	"github.com/gin-gonic/gin"
)

func ProductsRoutes(router *gin.Engine) {
	products := router.Group("/products")

	{
		products.GET("", controllers.GetProducts)
		products.POST("", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.CreateProduct)
		products.GET("/:id", controllers.GetProduct)
		products.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.UpdateProduct)
		products.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.DeleteProduct)
		products.PATCH("/:id/variant", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.UpdateVariant)
		// products.GET("/:id/images", controllers.GetProductImages)

	}
}
