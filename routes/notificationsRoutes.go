package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func NotificationsRoutes(router *gin.Engine) {
	notifications := router.Group("/v1/notifications")
	notifications.Use(middleware.RequireAuthentication, middleware.RequireShopAccess())
	{
		notifications.GET("", controllers.GetMyNotifications)
		notifications.GET("/unread-count", controllers.GetUnreadNotificationCount)
		notifications.PATCH("/:id/read", controllers.MarkNotificationRead)
		notifications.PATCH("/read-all", controllers.MarkAllNotificationsRead)
	}
}
