package realtime

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Message struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

var (
	clients   = make(map[*websocket.Conn]bool)
	clientMu  sync.Mutex
	Broadcast = make(chan Message)
)

func RegisterClient(conn *websocket.Conn) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clients[conn] = true
}

func UnregisterClient(conn *websocket.Conn) {
	clientMu.Lock()
	defer clientMu.Unlock()
	delete(clients, conn)
	conn.Close()
}

func StartHub() {
	for {
		msg := <-Broadcast
		clientMu.Lock()
		for client := range clients {
			if err := client.WriteJSON(msg); err != nil {
				client.Close()
				delete(clients, client)
			}
		}
		clientMu.Unlock()
	}
}
