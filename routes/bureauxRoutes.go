package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func BureauxRoutes(router *gin.Engine) {
	bureaux := router.Group("/v1/shops/:shopId/bureaux")
	bureaux.Use(middleware.RequireAuthentication)
	{
		// All shop roles can read: confirmation staff create orders and need
		// the stopdesk dropdown, same reasoning as GET delivery-rates.
		bureaux.GET("", middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.ListBureaux)
		bureaux.POST("", middleware.RequireShopAccess(), middleware.RequireShopPermission("bureaux.edit"), controllers.CreateBureau)
		bureaux.DELETE("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("bureaux.edit"), controllers.DeleteBureau)
	}
}
