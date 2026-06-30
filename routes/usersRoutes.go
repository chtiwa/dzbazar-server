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
		// Any shop member can view their own record (IndexUserByShop enforces self-or-Owner).
		shopUsers.GET("/:id", controllers.IndexUserByShop)

		owned := shopUsers.Group("")
		owned.Use(middleware.RequireRoles("Owner"))
		{
			owned.GET("", controllers.GetUsersByShop)
			owned.POST("", controllers.CreateUserByShop)
			owned.PATCH("/:id", controllers.UpdateUserByShop)
			owned.DELETE("/:id", controllers.DeleteUserByShop)
		}
	}
}
