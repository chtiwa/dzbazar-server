package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PlansRoutes(router *gin.Engine) {
	// Global plan catalog — read is public (pricing page), writes moved to
	// /v1/super-admin/plans (see superAdminRoutes.go). This used to require
	// only RequireAuthentication with no role check at all — any logged-in
	// user, including a confirmation courier account, could rewrite the global
	// plan catalog.
	plans := router.Group("/v1/plans")
	{
		plans.GET("", middleware.RateLimitByIP("plans", 30, time.Minute), controllers.GetPlans)
	}

	// Per-shop subscription
	sub := router.Group("/v1/shops/:shopId/subscription")
	sub.Use(middleware.RequireAuthentication)
	{
		sub.GET("", middleware.RequireShopAccess(), middleware.RequireShopPermission("subscription.view"), controllers.GetShopSubscription)
		sub.POST("", middleware.RequireShopAccess(), middleware.RequireShopPermission("subscription.edit"), controllers.SubscribeShopToPlan)
		sub.DELETE("", middleware.RequireShopAccess(), middleware.RequireShopPermission("subscription.edit"), controllers.CancelShopSubscription)
	}
}
