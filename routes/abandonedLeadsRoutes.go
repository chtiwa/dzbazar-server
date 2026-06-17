package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func AbandonedLeadsRoutes(router *gin.Engine) {
	leads := router.Group("/v1/shops/:shopId/abandoned-leads")
	{
		leads.POST("", middleware.OrderIPRateLimit(), controllers.CreateAbandonedLead)
		leads.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.GetAbandonedLeadsByShopID)
		leads.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.DeleteAbandonedLead)
	}
}
