package routes

import (
	"github.com/chtiwa/herbs-store-client/controllers"
	"github.com/gin-gonic/gin"
)

func ProductsRoutes(router *gin.Engine) {
	products := router.Group("/products")

	{
		products.GET("", controllers.GetProducts)
		products.POST("", controllers.CreateProduct)
		products.GET("/:id", controllers.GetProduct)
		products.PATCH("/:id", controllers.UpdateProduct)
		products.DELETE("/:id", controllers.DeleteProduct)
		products.POST("/:id/variant", controllers.CreateVariant)

	}
}
