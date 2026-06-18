package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/controllers/superadmin"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

// SuperAdminRoutes registers the entire /v1/super-admin namespace. Every
// route requires RequireAuthentication + RequireSuperAdmin at minimum; a few
// read-only sub-groups additionally allow the "support" platform role.
func SuperAdminRoutes(router *gin.Engine) {
	admin := router.Group("/v1/super-admin")
	admin.Use(middleware.RequireAuthentication, middleware.RequireSuperAdmin())
	{
		admin.GET("/dashboard/kpis", superadmin.GetKPIs)

		admin.GET("/shops", superadmin.ListShops)
		admin.GET("/shops/:id", superadmin.GetShop)
		admin.PATCH("/shops/:id/status", superadmin.UpdateShopStatus)
		admin.DELETE("/shops/:id", superadmin.DeleteShopByAdmin)
		admin.POST("/shops/:id/impersonate", superadmin.StartImpersonation)
		admin.POST("/impersonate/exit", superadmin.EndImpersonation)
		admin.PUT("/shops/:id/subscription", superadmin.SetShopSubscription)

		admin.GET("/users", superadmin.ListUsers)
		admin.GET("/users/:id", superadmin.GetUser)
		admin.PATCH("/users/:id/status", superadmin.UpdateUserStatus)
		admin.DELETE("/users/:id", superadmin.DeleteUserByAdmin)

		admin.GET("/products", superadmin.ListProducts)
		admin.GET("/orders", superadmin.ListOrders)
		admin.GET("/subscriptions", superadmin.ListSubscriptions)

		admin.GET("/plans", controllers.GetPlans)
		admin.POST("/plans", controllers.CreatePlan)
		admin.PATCH("/plans/:id", controllers.UpdatePlan)
		admin.DELETE("/plans/:id", controllers.DeletePlan)

		admin.GET("/feature-flags", superadmin.ListFeatureFlags)
		admin.POST("/feature-flags", superadmin.CreateFeatureFlag)
		admin.PATCH("/feature-flags/:id", superadmin.UpdateFeatureFlag)

		admin.GET("/settings", superadmin.ListSettings)
		admin.PATCH("/settings/:key", superadmin.UpsertSetting)
	}

	// Accessible to both super_admin and support — mounted as a second group
	// instead of relaxing the group-level middleware above. Support agents can
	// triage tickets and read the audit trail, but nothing else in this file.
	supportAccessible := router.Group("/v1/super-admin")
	supportAccessible.Use(middleware.RequireAuthentication, middleware.RequirePlatformRole("super_admin", "support"))
	{
		supportAccessible.GET("/audit-logs", superadmin.ListAuditLogs)

		supportAccessible.GET("/support-tickets", superadmin.ListSupportTickets)
		supportAccessible.GET("/support-tickets/:id", superadmin.GetSupportTicket)
		supportAccessible.POST("/support-tickets", superadmin.CreateSupportTicket)
		supportAccessible.PATCH("/support-tickets/:id", superadmin.UpdateSupportTicket)
		supportAccessible.POST("/support-tickets/:id/messages", superadmin.AddTicketMessage)
	}
}
