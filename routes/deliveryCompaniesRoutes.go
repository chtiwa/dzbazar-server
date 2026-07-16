package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DeliveryCompaniesRoutes(router *gin.Engine) {
	// Global available delivery companies — read-only here; writes are
	// super-admin-only and live under /v1/super-admin/delivery-companies/available.
	available := router.Group("/v1/delivery-companies/available")
	{
		available.GET("", middleware.RateLimitByIP("delivery-companies-available", 60, time.Minute), controllers.GetAvailableDeliveryCompanies)
	}

	// Per-shop integrations (credentials)
	integrations := router.Group("/v1/shops/:shopId/delivery-companies")
	integrations.Use(middleware.RequireAuthentication)
	{
		integrations.GET("", middleware.RequireShopAccess(), middleware.RequireShopPermission("delivery_companies.view"), controllers.GetShopDeliveryCompanies)
		integrations.POST("", middleware.RequireShopAccess(), middleware.RequireShopPermission("delivery_companies.edit"), controllers.ConnectDeliveryCompany)
		integrations.PATCH("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("delivery_companies.edit"), controllers.UpdateDeliveryCompanyCredentials)
		integrations.DELETE("/:id", middleware.RequireShopAccess(), middleware.RequireShopPermission("delivery_companies.edit"), controllers.DisconnectDeliveryCompany)
	}
}
