package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/middleware"
	"github.com/chtiwa/herbs-store-client/migrate"
	"github.com/chtiwa/herbs-store-client/realtime"
	"github.com/chtiwa/herbs-store-client/routes"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	migrate.Migrate()
}

// EventHandler is a function type for handling events
type EventHandler func(conn *websocket.Conn, data interface{})

// EventRouter maps event names to handler functions
type EventRouter struct {
	handlers map[string]EventHandler
}

// NewEventRouter initializes an EventRouter
func NewEventRouter() *EventRouter {
	return &EventRouter{handlers: make(map[string]EventHandler)}
}

// On registers an event and its handler
func (er *EventRouter) On(event string, handler EventHandler) {
	er.handlers[event] = handler
}

// Handle routes the incoming event to the appropriate handler
func (er *EventRouter) Handle(conn *websocket.Conn, event string, data interface{}) {
	if handler, ok := er.handlers[event]; ok {
		handler(conn, data)
	} else {
		// Handle unknown events
		conn.WriteMessage(websocket.TextMessage, []byte("Unknown event: "+event))
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		allowedOrigins := []string{os.Getenv("CLIENT_URL"), os.Getenv("CLIENT_URL_V2")}
		origin := r.Header.Get("Origin")
		for _, o := range allowedOrigins {
			if origin == o {
				return true
			}
		}
		return false
	},
}

var clients = make(map[*websocket.Conn]bool)
var clientsMutex = &sync.Mutex{}

func handleWebSocket(c *gin.Context, router *EventRouter) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("Error upgrading to WebSocket:", err)
		return
	}
	defer conn.Close()

	realtime.RegisterClient(conn)
	defer realtime.UnregisterClient(conn)
	for {
		// Read incoming message
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error reading message:", err)
			break
		}

		// Parse the incoming JSON message
		var incoming struct {
			Event string      `json:"event"`
			Data  interface{} `json:"data"`
		}
		err = json.Unmarshal(message, &incoming)
		if err != nil {
			fmt.Println("Error parsing message:", err)
			continue
		}

		// Route the event to the appropriate handler
		router.Handle(conn, incoming.Event, incoming.Data)
	}
}

func Broadcast(event string, payload interface{}) {
	message := map[string]interface{}{
		"event": event,
		"data":  payload,
	}
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		fmt.Println("Error marshaling message", err)
		return
	}

	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	for conn := range clients {
		err := conn.WriteMessage(websocket.TextMessage, jsonMessage)
		if err != nil {
			fmt.Println("Error while sending  message to client: ", err)
			conn.Close()
			delete(clients, conn)
		}
	}
}

func main() {
	router := gin.Default()

	router.Use(middleware.CORSMiddleware())

	// routes
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)

	// websockets
	// Initialize the event router
	WsRouter := NewEventRouter()
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
	router.GET("/", func(c *gin.Context) {
		handleWebSocket(c, WsRouter)
	})

	fmt.Println("The server is running successfully!")

	router.Run()
}
