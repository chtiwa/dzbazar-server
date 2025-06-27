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
		products.POST("", middleware.RequireAuthentication, controllers.CreateProduct)
		products.GET("/:id", middleware.RequireAuthentication, controllers.GetProduct)
		products.PATCH("/:id", middleware.RequireAuthentication, controllers.UpdateProduct)
		products.DELETE("/:id", middleware.RequireAuthentication, controllers.DeleteProduct)

	}
}
