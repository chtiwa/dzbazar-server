package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func UsersRoutes(router *gin.Engine) {
	users := router.Group("/api/v1/users")

	{
		users.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetUsersByShop)
		users.GET("/validate", middleware.RequireAuthentication, controllers.Validate)
		users.POST("/create", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.CreateUserByShop)
		users.POST("/login", controllers.Login)
		users.POST("/signup", controllers.SignUp)
		users.GET("/logout", controllers.Logout)
		// token verification => admin verification => action
		users.GET("/verify", middleware.RequireAuthentication)
		users.POST("/verify-otp", controllers.VerifyUser)
		users.GET("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.IndexUserByShop)
		users.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.UpdateUserByShop)
		users.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.DeleteUserByShop)

	}
}
