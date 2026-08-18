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
		// Pixel status changes what a merchant's live tracking setup actually
		// does, so it stays super_admin-only — same bar as shop suspend/delete
		// above. Pixels themselves are read via GetShop (preloaded, token-free).
		admin.PATCH("/shops/:id/pixels/:pixelId/status", superadmin.UpdateShopPixelStatus)
		admin.POST("/shops/:id/impersonate", superadmin.StartImpersonation)
		admin.POST("/impersonate/exit", superadmin.EndImpersonation)
		admin.PUT("/shops/:id/subscription", superadmin.SetShopSubscription)
		// Cancelling a subscription is a billing-affecting action, same bar as
		// the plan-set above, so it stays super_admin-only rather than moving
		// to supportAccessible below.
		admin.DELETE("/shops/:id/subscription", superadmin.CancelSubscription)

		admin.GET("/users", superadmin.ListUsers)
		admin.GET("/users/:id", superadmin.GetUser)
		admin.PATCH("/users/:id/status", superadmin.UpdateUserStatus)
		// PlatformRole grants/revokes access to this entire panel, so it
		// stays super_admin-only — same bar as shop delete/impersonate above.
		admin.PATCH("/users/:id/platform-role", superadmin.SetPlatformRole)
		admin.DELETE("/users/:id", superadmin.DeleteUserByAdmin)

		admin.GET("/products", superadmin.ListProducts)

		// Force-hide changes what a merchant's storefront actually shows, so
		// it stays super_admin-only — same bar as offers/coupons/landing-pages
		// force-actions above. The list itself stays read-only, per
		// ListProducts's doc comment.
		admin.PATCH("/products/:id/force-hide", superadmin.ForceHideProduct)

		admin.GET("/subscriptions", superadmin.ListSubscriptions)

		// Force-disable changes what a merchant's storefront actually shows,
		// so it stays super_admin-only — same bar as shop suspend/delete
		// above. The list itself is read-only and lives in supportAccessible
		// below, next to the other read-only cross-tenant lookups.
		admin.PATCH("/offers/:id/force-disable", superadmin.ForceDisableOffer)

		// Coupons: force-disable kills a live discount code a merchant is
		// actively running, same bar as offers force-disable above, so it
		// stays super_admin-only. The list itself is read-only and lives in
		// supportAccessible below, next to the other read-only cross-tenant
		// lookups.
		admin.PATCH("/coupons/:id/force-disable", superadmin.ForceDisableCoupon)

		// Landing pages: force-unpublish kills a live page a merchant is
		// actively running, same bar as offers/coupons force-disable above,
		// so it stays super_admin-only. The list itself is read-only and
		// lives in supportAccessible below, next to the other read-only
		// cross-tenant lookups.
		admin.PATCH("/landing-pages/:id/force-unpublish", superadmin.ForceUnpublishLandingPage)

		// Landing page experiments (A/B tests): force-stop halts a live test
		// and changes what visitors see next, same bar as landing-pages
		// force-unpublish above, so it stays super_admin-only. The list
		// itself is read-only and lives in supportAccessible below, next to
		// the other read-only cross-tenant lookups.
		admin.PATCH("/landing-page-experiments/:id/force-stop", superadmin.ForceStopExperiment)

		admin.GET("/plan-requests", superadmin.ListPlanSwitchRequests)
		admin.POST("/plan-requests/:id/approve", superadmin.ApprovePlanSwitchRequest)
		admin.POST("/plan-requests/:id/reject", superadmin.RejectPlanSwitchRequest)

		admin.GET("/plans", controllers.GetPlans)
		admin.POST("/plans", controllers.CreatePlan)
		admin.PATCH("/plans/:id", controllers.UpdatePlan)
		admin.DELETE("/plans/:id", controllers.DeletePlan)

		admin.GET("/delivery-companies/available", controllers.ListAllAvailableDeliveryCompanies)
		admin.POST("/delivery-companies/available", controllers.CreateAvailableDeliveryCompany)
		admin.PATCH("/delivery-companies/available/:id", controllers.UpdateAvailableDeliveryCompany)
		admin.DELETE("/delivery-companies/available/:id", controllers.DeleteAvailableDeliveryCompany)

		// Wilayas are a fixed set of 58 (edit-only, no create/delete) that
		// seed every shop's DeliveryRate on creation, so a rate/flag change
		// here affects delivery pricing platform-wide — same bar as the
		// delivery-companies writes above, so it stays super_admin-only.
		admin.GET("/wilayas", superadmin.ListWilayas)
		admin.PATCH("/wilayas/:id", superadmin.UpdateWilaya)

		admin.GET("/feature-flags", superadmin.ListFeatureFlags)
		admin.POST("/feature-flags", superadmin.CreateFeatureFlag)
		admin.PATCH("/feature-flags/:id", superadmin.UpdateFeatureFlag)
		// Deletion is safe unconditionally: IsFeatureEnabled fails open on a
		// missing key (see services/featureFlags.go), so removing even the
		// live coupons_enabled kill-switch can't strand a gate closed. Stays
		// super_admin-only, same bar as the writes above.
		admin.DELETE("/feature-flags/:id", superadmin.DeleteFeatureFlag)

		admin.GET("/settings", superadmin.ListSettings)
		admin.PATCH("/settings/:key", superadmin.UpsertSetting)
		admin.DELETE("/settings/:key", superadmin.DeleteSetting)

		// Role defaults are global (no shop_id) but take effect for every
		// shop the instant they change, so the write stays super_admin-only —
		// same bar as offers force-disable above. The catalog/deviations
		// reads live in supportAccessible below, next to the other read-only
		// cross-tenant lookups.
		admin.PATCH("/rbac/role-defaults/:role/:action", superadmin.UpdateRoleDefault)
	}

	// Accessible to both super_admin and support — mounted as a second group
	// instead of relaxing the group-level middleware above. Support agents can
	// triage tickets, read the audit trail, look up orders read-only, and
	// look up/ban clients to resolve a ticket — nothing else in this file.
	supportAccessible := router.Group("/v1/super-admin")
	supportAccessible.Use(middleware.RequireAuthentication, middleware.RequirePlatformRole("super_admin", "support"))
	{
		supportAccessible.GET("/audit-logs", superadmin.ListAuditLogs)

		// RBAC catalog: the permission_actions/role_action_defaults matrix is
		// global, non-shop-scoped data, and the shop-member deviation list is
		// read-only diagnostics — same reasoning as audit-logs above.
		supportAccessible.GET("/rbac/catalog", superadmin.GetPermissionCatalog)
		supportAccessible.GET("/rbac/deviations", superadmin.ListRoleDeviations)

		// Orders are read-only here (see ListOrders/GetOrder doc comments) —
		// support needs these to resolve tickets against a real order, same
		// reasoning as audit-logs above.
		supportAccessible.GET("/orders", superadmin.ListOrders)
		supportAccessible.GET("/orders/:id", superadmin.GetOrder)

		// Offers: read-only cross-tenant list so support/ops can spot an
		// abusive or deceptive promotion without per-shop access. Same
		// reasoning as orders above — the mutation (force-disable) is the
		// one gated to super_admin only, up in the admin group.
		supportAccessible.GET("/offers", superadmin.ListOffers)

		// Coupons: read-only cross-tenant list so support/ops can spot an
		// abusive or leaked discount code without per-shop access. Same
		// reasoning as offers above — the mutation (force-disable) is the
		// one gated to super_admin only, up in the admin group.
		supportAccessible.GET("/coupons", superadmin.ListCoupons)

		// Landing pages: read-only cross-tenant list so support/ops can spot
		// an abusive or deceptive page without per-shop access. Same
		// reasoning as offers/coupons above — the mutation (force-unpublish)
		// is the one gated to super_admin only, up in the admin group.
		supportAccessible.GET("/landing-pages", superadmin.ListLandingPages)

		// Landing page experiments (A/B tests): read-only cross-tenant list so
		// support/ops can spot a runaway or abandoned test without per-shop
		// access. Same reasoning as offers/coupons/landing-pages above — the
		// mutation (force-stop) is the one gated to super_admin only, up in
		// the admin group.
		supportAccessible.GET("/landing-page-experiments", superadmin.ListExperiments)

		// Clients: read-only lookup plus the ban toggle. Ban/unban is the one
		// mutation support needs to actually resolve a fraud/harassment
		// ticket without escalating to a super admin — reversible (unlike
		// delete), so it's fine at this role, same reasoning as the orders
		// read access above.
		supportAccessible.GET("/clients", superadmin.ListClients)
		supportAccessible.GET("/clients/:id", superadmin.GetClient)
		supportAccessible.PATCH("/clients/:id/ban", superadmin.ToggleClientBan)

		// Flagged clients (fraud review): read-only list plus resolve/dismiss.
		// Resolving doesn't delete the flag (history is kept) and doesn't ban
		// anyone new — it only stops an existing flag from blocking future
		// orders — so it's fine at this role, same reasoning as the client
		// ban toggle above.
		supportAccessible.GET("/flagged-clients", superadmin.ListFlaggedClients)
		supportAccessible.PATCH("/flagged-clients/:id/resolve", superadmin.ResolveFlaggedClient)

		supportAccessible.GET("/support-tickets", superadmin.ListSupportTickets)
		supportAccessible.GET("/support-tickets/:id", superadmin.GetSupportTicket)
		supportAccessible.POST("/support-tickets", superadmin.CreateSupportTicket)
		supportAccessible.PATCH("/support-tickets/:id", superadmin.UpdateSupportTicket)
		supportAccessible.POST("/support-tickets/:id/messages", superadmin.AddTicketMessage)
	}
}
