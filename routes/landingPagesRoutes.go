package routes

import (
	"github.com/chtiwa/lk-parfumo-server/controllers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/gin-gonic/gin"
)

func LandingPagesRoutes(router *gin.Engine) {
	landingPages := router.Group("/landing-pages")
	{
		landingPages.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetLandingPages)
		landingPages.POST("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateLandingPage)
		landingPages.GET("/:id", controllers.IndexLandingPage)
		landingPages.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteLandingPage)
		landingPages.PATCH("/:id/images", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateLandingPageImages)
		landingPages.DELETE("/:id/images/:imageId", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteLandingPageImage)
	}
}
