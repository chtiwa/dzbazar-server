package realtime

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("websocket upgrade error:", err)
		return
	}

	RegisterClient(conn)

	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			UnregisterClient(conn)
			break
		}
		Broadcast <- msg
	}
}
