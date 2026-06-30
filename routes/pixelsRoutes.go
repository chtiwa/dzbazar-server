package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PixelsRoutes(router *gin.Engine) {
	// Public route for marketplace / storefront usage
	router.GET(
		"/v1/shops/:shopId/pixels/active",
		controllers.IndexActivePixelByShop,
	)

	// Protected merchant routes
	pixels := router.Group("/v1/shops/:shopId/pixels")
	pixels.Use(middleware.RequireAuthentication)
	{
		pixels.GET("", middleware.RequireRoles("Owner"), controllers.GetPixelsByShop)
		pixels.GET("/:id", middleware.RequireRoles("Owner"), controllers.IndexPixel)
		pixels.POST("", middleware.RequireRoles("Owner"), controllers.CreatePixel)
		pixels.PATCH("/:id", middleware.RequireRoles("Owner"), controllers.UpdatePixel)
		pixels.DELETE("/:id", middleware.RequireRoles("Owner"), controllers.DeletePixel)
	}
}
