package routes

import (
	ws "github.com/chtiwa/dzbazar-server/realtime"
	"github.com/gin-gonic/gin"
)

func WebSocketRoutes(router *gin.Engine) {
	router.GET("/ws", ws.WebSocketHandler)
}
