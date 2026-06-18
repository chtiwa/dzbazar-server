package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PlansRoutes(router *gin.Engine) {
	// Global plan catalog — read is public (pricing page), writes moved to
	// /v1/super-admin/plans (see superAdminRoutes.go). This used to require
	// only RequireAuthentication with no role check at all — any logged-in
	// user, including a Logistics courier account, could rewrite the global
	// plan catalog.
	plans := router.Group("/v1/plans")
	{
		plans.GET("", controllers.GetPlans)
	}

	// Per-shop subscription
	sub := router.Group("/v1/shops/:shopId/subscription")
	sub.Use(middleware.RequireAuthentication)
	{
		sub.GET("", controllers.GetShopSubscription)
		sub.POST("", middleware.RequireShopAccess("Owner"), controllers.SubscribeShopToPlan)
		sub.DELETE("", middleware.RequireShopAccess("Owner"), controllers.CancelShopSubscription)
	}
}
