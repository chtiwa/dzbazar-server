package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DeliveryCompaniesRoutes(router *gin.Engine) {
	// Global available delivery companies — read-only here; writes are
	// super-admin-only and live under /v1/super-admin/delivery-companies/available.
	available := router.Group("/v1/delivery-companies/available")
	{
		available.GET("", controllers.GetAvailableDeliveryCompanies)
	}

	// Per-shop integrations (credentials)
	integrations := router.Group("/v1/shops/:shopId/delivery-companies")
	integrations.Use(middleware.RequireAuthentication)
	{
		integrations.GET("", controllers.GetShopDeliveryCompanies)
		integrations.POST("", middleware.RequireShopAccess("owner", "moderator"), controllers.ConnectDeliveryCompany)
		integrations.PATCH("/:id", middleware.RequireShopAccess("owner", "moderator"), controllers.UpdateDeliveryCompanyCredentials)
		integrations.DELETE("/:id", middleware.RequireShopAccess("owner"), controllers.DisconnectDeliveryCompany)
	}
}
