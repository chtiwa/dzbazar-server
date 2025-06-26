package routes

import (
	ws "github.com/chtiwa/herbs-store-client/realtime"
	"github.com/gin-gonic/gin"
)

func WebSocketRoutes(router *gin.Engine) {
	router.GET("/ws", ws.WebSocketHandler)
}
