package routes

import (
	"github.com/chtiwa/lk-parfumo-server/controllers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/gin-gonic/gin"
)

func DashboardRoutes(router *gin.Engine) {
	dashboard := router.Group("/dashboard")

	{
		dashboard.GET("/orders", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetOrdersDashboard)
	}
}
