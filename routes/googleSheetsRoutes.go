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
		sheets.GET("", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.view"), controllers.GetGoogleSheets)
		sheets.PUT("/:kind", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.SaveGoogleSheet)
		sheets.DELETE("/:kind", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.DisconnectGoogleSheet)
		sheets.POST("/:kind/test", middleware.RequireShopAccess(), middleware.RequireShopPermission("sheets.edit"), controllers.TestGoogleSheet)
	}
}
