package routes

import "github.com/gin-gonic/gin"

func UsersRoutes(router *gin.Engine) {
	users := router.Group("/users")

	{
		users.GET("")
	}
}
