package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func ConfirmatricesRoutes(router *gin.Engine) {
	router.GET(
		"/v1/shops/:shopId/confirmatrices/rates",
		middleware.RequireAuthentication,
		middleware.RequireShopAccess("owner", "moderator"),
		controllers.GetConfirmationRates,
	)
}
