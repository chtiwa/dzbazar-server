package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func LandingPagesRoutes(router *gin.Engine) {
	landingPages := router.Group("/landing-pages")
	{
		landingPages.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Admin", "Moderator"), controllers.GetLandingPages)
		landingPages.POST("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateLandingPage)
		landingPages.GET("/:id", controllers.IndexLandingPage)
		landingPages.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteLandingPage)
		landingPages.PATCH("/:id/images", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateLandingPage)
		landingPages.DELETE("/:id/images/:imageId", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteLandingPageImage)
	}
}
