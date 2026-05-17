package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ShopsRoutes(router *gin.Engine) {
	shops := router.Group("/shops")
	{
		shops.POST("", middleware.RequireAuthentication, controllers.CreateShop)
		shops.PATCH("/:shopId", middleware.RequireAuthentication, controllers.UpdateShop)
		shops.DELETE("/:shopId", middleware.RequireAuthentication, controllers.DeleteShop)
	}
}
