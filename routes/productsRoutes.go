package routes

import (
	"github.com/chtiwa/lk-parfumo-server/controllers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/gin-gonic/gin"
)

func ProductsRoutes(router *gin.Engine) {
	products := router.Group("/products")

	{
		products.GET("", controllers.GetProducts)
		products.POST("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateProduct)
		products.GET("/:id", controllers.GetProduct)
		products.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateProduct)
		products.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteProduct)
		products.PATCH("/variant/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateVariant)
		products.PATCH("/images/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateProductImages)
		products.GET("/client", controllers.GetProductsClient)
		products.GET("/search", controllers.GetProductsBySearch)
		products.GET("/tags", controllers.GetTags)
		products.GET("/all-tags", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetAllTags)
		products.DELETE("/tags/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteTag)
		products.POST("/tags", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateTag)
		products.GET("/promo", controllers.GetPromoRemaining)
	}
}
