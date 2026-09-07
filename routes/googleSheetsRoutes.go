package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func GoogleSheetsRoutes(router *gin.Engine) {
	sheets := router.Group("/v1/shops/:shopId/sheets")
	sheets.Use(middleware.RequireAuthentication)
	{
		sheets.GET("", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.view"), controllers.GetGoogleSheetsStatus)
		sheets.POST("", middleware.RequireShopAccess("owner", "moderator"), controllers.ConnectGoogleSheets)
		sheets.PATCH("", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.UpdateGoogleSheetsCredentials)
		sheets.DELETE("", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.DisconnectGoogleSheets)
		sheets.POST("/test", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.TestGoogleSheetsConnection)
	}
}
