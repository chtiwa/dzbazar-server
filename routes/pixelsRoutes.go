package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PixelsRoutes(router *gin.Engine) {
	// Public route for marketplace / storefront usage
	router.GET(
		"/v1/shops/:shopId/pixels/active",
		middleware.RateLimitByIP("pixels-active", 60, time.Minute),
		controllers.IndexActivePixelByShop,
	)

	// Protected merchant routes
	pixels := router.Group("/v1/shops/:shopId/pixels")
	pixels.Use(middleware.RequireAuthentication)
	{
		pixels.GET("", middleware.RequireShopAccess(), middleware.RequireShopPermission("pixels.view"), controllers.GetPixelsByShop)
		pixels.GET("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("pixels.view"), controllers.IndexPixel)
		pixels.POST("", middleware.RequireShopAccess("owner", "moderator"), controllers.CreatePixel)
		pixels.PATCH("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("pixels.edit"), controllers.UpdatePixel)
		pixels.DELETE("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("pixels.delete"), controllers.DeletePixel)
	}
}
