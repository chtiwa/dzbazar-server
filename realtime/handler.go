package realtime

import (
	"fmt"
	"net/http"

	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func WebSocketHandler(c *gin.Context) {
	shopID := c.Query("shopId")
	if shopID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing shopId"})
		return
	}

	userVal, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}
	user, ok := userVal.(models.User)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	if _, hasAccess := middleware.GetShopMembership(user.ID, shopID); !hasAccess {
		c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to access this store workspace"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("websocket upgrade error:", err)
		return
	}

	RegisterClient(conn, shopID)

	// Dashboards are receive-only; this loop exists solely to detect
	// disconnects (a client-sent payload would otherwise be discarded by
	// ReadJSON's caller doing nothing with it, but we don't echo it back
	// into Broadcast — that would let any connected client inject fake
	// events to every other dashboard on its shop).
	for {
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			UnregisterClient(conn)
			break
		}
	}
}
