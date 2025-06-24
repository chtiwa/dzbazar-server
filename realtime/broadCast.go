package realtime

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

var clients = make(map[*websocket.Conn]bool)
var ClientsMutex = &sync.Mutex{}

func RegisterClient(conn *websocket.Conn) {
	ClientsMutex.Lock()
	defer ClientsMutex.Unlock()
	clients[conn] = true
}

func UnregisterClient(conn *websocket.Conn) {
	ClientsMutex.Lock()
	defer ClientsMutex.Unlock()
	delete(clients, conn)
}

func Broadcast(event string, data interface{}) {
	message := map[string]interface{}{
		"event": event,
		"data":  data,
	}
	jsonData, err := json.Marshal(message)
	if err != nil {
		fmt.Println("Error marshaling event:", err)
		return
	}

	ClientsMutex.Lock()
	defer ClientsMutex.Unlock()
	for client := range clients {
		err := client.WriteMessage(websocket.TextMessage, jsonData)
		if err != nil {
			fmt.Println("Error sending message to client:", err)
			client.Close()
			delete(clients, client)
		}
	}
}
