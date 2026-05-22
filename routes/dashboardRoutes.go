package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DashboardRoutes(router *gin.Engine) {
	dashboard := router.Group("/api/v1/dashboard")

	{
		dashboard.GET("/orders", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetOrdersDashboard)
	}
}
