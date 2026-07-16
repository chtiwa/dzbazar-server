package routes

import (
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func UsersRoutes(router *gin.Engine) {
	users := router.Group("/v1/users")
	{
		users.POST("/login", middleware.RateLimitByIP("login", 30, 15*time.Minute), controllers.Login)
		users.POST("/signup", middleware.RateLimitByIP("signup", 5, time.Hour), controllers.SignUp)
		users.GET("/logout", controllers.Logout)
		users.POST("/verify-otp", middleware.RateLimitByIP("verify-otp", 10, 15*time.Minute), controllers.VerifyUser)
		users.POST("/forgot-password", middleware.RateLimitByIP("forgot-password", 5, time.Hour), controllers.ForgotPassword)
		users.POST("/reset-password", middleware.RateLimitByIP("reset-password", 10, 15*time.Minute), controllers.ResetPassword)

		users.GET("/validate", middleware.RequireAuthentication, controllers.Validate)
		users.GET("/verify", middleware.RequireAuthentication)
	}

	shopUsers := router.Group("/v1/shops/:shopId/users")
	shopUsers.Use(middleware.RequireAuthentication)
	{
		// Any shop member can view their own record (IndexUserByShop enforces self-or-owner).
		shopUsers.GET("/:id", middleware.RequireShopAccess(), controllers.IndexUserByShop)

		// each independently gated so overrides can grant/deny per action
		manageable := shopUsers.Group("")
		manageable.Use(middleware.RequireShopAccess())
		{
			manageable.GET("", middleware.RequireShopPermission("users.view"), controllers.GetUsersByShop)
			manageable.POST("", middleware.RequireShopPermission("users.create"), controllers.CreateUserByShop)
			manageable.PATCH("/:id", middleware.RequireShopPermission("users.edit"), controllers.UpdateUserByShop)
			manageable.DELETE("/:id", middleware.RequireShopPermission("users.delete"), controllers.DeleteUserByShop)
		}

		// only owner can manage per-member permission overrides
		permissions := shopUsers.Group("/:id/permissions")
		permissions.Use(middleware.RequireShopAccess("owner"))
		{
			permissions.GET("", controllers.GetMemberPermissions)
			permissions.PUT("/:action", controllers.SetMemberPermission)
			permissions.DELETE("/:action", controllers.DeleteMemberPermission)
		}
	}
}
