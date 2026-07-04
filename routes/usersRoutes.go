package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func UsersRoutes(router *gin.Engine) {
	users := router.Group("/v1/users")
	{
		users.POST("/login", controllers.Login)
		users.POST("/signup", controllers.SignUp)
		users.GET("/logout", controllers.Logout)
		users.POST("/verify-otp", controllers.VerifyUser)
		users.POST("/forgot-password", controllers.ForgotPassword)
		users.POST("/reset-password", controllers.ResetPassword)

		users.GET("/validate", middleware.RequireAuthentication, controllers.Validate)
		users.GET("/verify", middleware.RequireAuthentication)
	}

	shopUsers := router.Group("/v1/shops/:shopId/users")
	shopUsers.Use(middleware.RequireAuthentication)
	{
		// Any shop member can view their own record (IndexUserByShop enforces self-or-owner).
		shopUsers.GET("/:id", controllers.IndexUserByShop)

		// owner and moderator can list, create, and edit members
		manageable := shopUsers.Group("")
		manageable.Use(middleware.RequireRoles("owner", "moderator"))
		{
			manageable.GET("", controllers.GetUsersByShop)
			manageable.POST("", controllers.CreateUserByShop)
			manageable.PATCH("/:id", controllers.UpdateUserByShop)
		}

		// only owner can remove members
		deletable := shopUsers.Group("")
		deletable.Use(middleware.RequireRoles("owner"))
		{
			deletable.DELETE("/:id", controllers.DeleteUserByShop)
		}
	}
}
