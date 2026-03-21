package routes

import (
	"github.com/chtiwa/lk-parfumo-server/controllers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/gin-gonic/gin"
)

func UsersRoutes(router *gin.Engine) {
	users := router.Group("/users")

	{
		users.GET("", middleware.RequireAuthentication, middleware.RequireRoles("Admin"), controllers.GetUsers)
		users.GET("/validate", middleware.RequireAuthentication, controllers.Validate)
		users.POST("/create", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.CreateUser)
		users.POST("/login", controllers.Login)
		users.GET("/logout", controllers.Logout)
		// token verification => admin verification => action
		users.GET("/verify", middleware.RequireAuthentication, middleware.RequireAdmin)
		users.PATCH("/:id", middleware.RequireAuthentication, controllers.UpdateUser)
		// users.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireAdmin, controllers.DeleteUser)

	}
}
