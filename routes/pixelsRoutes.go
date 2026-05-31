package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PixelsRoutes(router *gin.Engine) {
	// Public route for marketplace / storefront usage
	router.GET(
		"/api/v1/shops/:shopId/pixels/active",
		controllers.IndexActivePixelByShop,
	)

	// Protected merchant routes
	pixels := router.Group("/api/v1/shops/:shopId/pixels")
	pixels.Use(middleware.RequireAuthentication)
	{
		pixels.GET("", middleware.RequireRoles("Owner", "Staff"), controllers.GetPixelsByShop)
		pixels.GET("/:id", middleware.RequireRoles("Owner", "Staff"), controllers.IndexPixel)
		pixels.POST("", middleware.RequireRoles("Owner"), controllers.CreatePixel)
		pixels.PATCH("/:id", middleware.RequireRoles("Owner"), controllers.UpdatePixel)
		pixels.DELETE("/:id", middleware.RequireRoles("Owner"), controllers.DeletePixel)
	}
}
