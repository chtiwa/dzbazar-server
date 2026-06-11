package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func LandingPagesRoutes(router *gin.Engine) {
	adminShop := router.Group("/v1/shops")
	{
		adminLandingPages := adminShop.Group("/:shopId/landing-pages")
		{
			adminLandingPages.POST("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.CreateLandingPageByShop)
			adminLandingPages.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.GetLandingPagesByShop)
			adminLandingPages.GET("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.GetLandingPageByShop)

			adminLandingPages.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner", "Staff"), controllers.UpdateLandingPageByShop)
			adminLandingPages.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Owner"), controllers.DeleteLandingPageByShop)

		}
	}

	publicLandingPages := adminShop.Group("/landing-pages")
	{
		publicLandingPages.GET("/:id", controllers.IndexLandingPage)
	}
}
