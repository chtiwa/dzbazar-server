package routes

import (
	"github.com/chtiwa/herbs-store-client/controllers"
	"github.com/gin-gonic/gin"
)

func CategoriesRoutes(router *gin.Engine) {
	categories := router.Group("/categories")

	{
		categories.GET("", controllers.GetCategories)
	}
}
