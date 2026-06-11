package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func DeliveryCompaniesRoutes(router *gin.Engine) {
	// Global available delivery companies (admin-managed, read is public)
	available := router.Group("/v1/delivery-companies/available")
	{
		available.GET("", controllers.GetAvailableDeliveryCompanies)
		available.POST("", middleware.RequireAuthentication, controllers.CreateAvailableDeliveryCompany)
		available.PATCH("/:id", middleware.RequireAuthentication, controllers.UpdateAvailableDeliveryCompany)
		available.DELETE("/:id", middleware.RequireAuthentication, controllers.DeleteAvailableDeliveryCompany)
	}

	// Per-shop integrations (credentials)
	integrations := router.Group("/v1/shops/:shopId/delivery-companies")
	integrations.Use(middleware.RequireAuthentication)
	{
		integrations.GET("", controllers.GetShopDeliveryCompanies)
		integrations.POST("", middleware.RequireShopAccess("Owner"), controllers.ConnectDeliveryCompany)
		integrations.PATCH("/:id", middleware.RequireShopAccess("Owner"), controllers.UpdateDeliveryCompanyCredentials)
		integrations.DELETE("/:id", middleware.RequireShopAccess("Owner"), controllers.DisconnectDeliveryCompany)
	}
}
