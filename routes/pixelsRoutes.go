package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PixelsRoutes(router *gin.Engine) {
	pixels := router.Group("/pixels")
	{
		pixels.POST("", middleware.RequireAuthentication, controllers.CreatePixel)
		pixels.PATCH("/:id", middleware.RequireAuthentication, controllers.UpdatePixel)
		pixels.DELETE("/:id", middleware.RequireAuthentication, controllers.DeletePixel)
	}
}
