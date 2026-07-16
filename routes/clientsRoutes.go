package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ClientsRoutes(router *gin.Engine) {
	clients := router.Group("/v1/shops/:shopId/clients")
	{
		clients.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.GetClientsByShopID)
		clients.GET("/search", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.GetClientsBySearch)
		clients.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.CreateClientByShopID)
		clients.POST("/excel", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.UploadExcelClients)

		clients.GET("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator", "confirmation"), controllers.IndexClientByShopID)
		clients.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("clients.edit"), controllers.UpdateClientByShopID)
		clients.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess(), middleware.RequireShopPermission("clients.delete"), controllers.DeleteClientByShopID)
	}
}
