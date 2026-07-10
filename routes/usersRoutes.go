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
		users.POST("/login", middleware.RateLimitByIP("login", 10, 15*time.Minute), controllers.Login)
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

		// owner and moderator can list, create, and edit members
		manageable := shopUsers.Group("")
		manageable.Use(middleware.RequireShopAccess("owner", "moderator"))
		{
			manageable.GET("", controllers.GetUsersByShop)
			manageable.POST("", controllers.CreateUserByShop)
			manageable.PATCH("/:id", controllers.UpdateUserByShop)
		}

		// only owner can remove members
		deletable := shopUsers.Group("")
		deletable.Use(middleware.RequireShopAccess("owner"))
		{
			deletable.DELETE("/:id", controllers.DeleteUserByShop)
		}
	}
}
