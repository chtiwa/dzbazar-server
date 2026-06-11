package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func PlansRoutes(router *gin.Engine) {
	// Global plan catalog — read is public, write is authenticated (admin)
	plans := router.Group("/v1/plans")
	{
		plans.GET("", controllers.GetPlans)
		plans.POST("", middleware.RequireAuthentication, controllers.CreatePlan)
		plans.PATCH("/:id", middleware.RequireAuthentication, controllers.UpdatePlan)
		plans.DELETE("/:id", middleware.RequireAuthentication, controllers.DeletePlan)
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
