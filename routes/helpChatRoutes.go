package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func HelpChatRoutes(router *gin.Engine) {
	router.POST(
		"/v1/shops/:shopId/help-chat",
		middleware.RequireAuthentication,
		middleware.RequireShopAccess("owner", "moderator", "confirmation"),
		middleware.RateLimitByShop("help-chat", 20, time.Minute),
		controllers.AskHelpChat,
	)
}
