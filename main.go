package main

import (
	"fmt"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/middleware"
	"github.com/chtiwa/herbs-store-client/migrate"
	"github.com/chtiwa/herbs-store-client/routes"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	migrate.Migrate()
}

func main() {
	router := gin.Default()

	router.Use(middleware.CORSMiddleware())

	// routes
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)

	// websockets
	// Initialize the event router
	WsRouter := initializers.NewEventRouter()
	WsRouter.On("greet", func(conn *websocket.Conn, data interface{}) {
		payload := data.(map[string]interface{})
		name := payload["name"].(string)
		conn.WriteMessage(websocket.TextMessage, []byte("Hello, "+name+"!"))
	})

	WsRouter.On("ping", func(conn *websocket.Conn, data interface{}) {
		// log.Println("Ping Ping")
		conn.WriteMessage(websocket.TextMessage, []byte("pong"))
	})

	// Define WebSocket endpoint
	router.GET("/ws", func(c *gin.Context) {
		initializers.HandleWebSocket(c, WsRouter)
	})

	fmt.Println("The server is running successfully!")

	router.Run()
}
