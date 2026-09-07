package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func AIImageToolRoutes(router *gin.Engine) {
	shop := router.Group("/v1/shops/:shopId")
	{
		shop.POST("/ai-image",
			middleware.RequireAuthentication,
			middleware.RequireShopAccess(),
			middleware.RequireShopPermission("ai_image_tool.use"),
			middleware.RateLimitByShop("ai-image-tool", 20, time.Minute),
			controllers.GenerateAIImage,
		)
	}
}
