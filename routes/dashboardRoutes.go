package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DashboardRoutes(router *gin.Engine) {
	dashboard := router.Group("/api/v1/shops/:shopId/dashboard")
	dashboard.Use(middleware.RequireAuthentication)
	{
		dashboard.GET("/orders", middleware.RequireRoles("Owner", "Staff", "Logistics"), controllers.GetOrdersDashboard)
	}
}
