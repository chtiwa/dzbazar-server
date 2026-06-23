package routes

import (
	"github.com/chtiwa/dzbazar-server/middleware"
	ws "github.com/chtiwa/dzbazar-server/realtime"
	"github.com/gin-gonic/gin"
)

func WebSocketRoutes(router *gin.Engine) {
	router.GET("/ws", middleware.RequireAuthentication, ws.WebSocketHandler)
}
