package initializers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/chtiwa/herbs-store-client/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

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

func HandleWebSocket(c *gin.Context, router *EventRouter) {
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
