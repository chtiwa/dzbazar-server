package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PixelsRoutes(router *gin.Engine) {
	pixels := router.Group("/api/v1/pixels")
	{
		pixels.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.CreatePixel)
		pixels.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.UpdatePixel)
		pixels.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("Owner"), controllers.DeletePixel)
	}
}
