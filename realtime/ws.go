package realtime

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gorilla/websocket"
)

const wsBroadcastChannel = "ws:broadcast"

type Message struct {
	Event  string      `json:"event"`
	ShopID string      `json:"shopId"`
	Data   interface{} `json:"data"`
}

var (
	clients   = make(map[*websocket.Conn]string) // conn -> shopID it's scoped to
	clientMu  sync.Mutex
	// Buffered so a burst of order events doesn't block senders on StartHub
	// keeping up; callers still guard the send with a timeout (see
	// controllers.processOrderEvent) in case the hub is stalled entirely.
	Broadcast = make(chan Message, 256)
)

func RegisterClient(conn *websocket.Conn, shopID string) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clients[conn] = shopID
}

func UnregisterClient(conn *websocket.Conn) {
	clientMu.Lock()
	defer clientMu.Unlock()
	delete(clients, conn)
	conn.Close()
}

// StartHub drains the in-process Broadcast channel and republishes each
// message to Redis. It never writes to the local clients map directly —
// StartSubscriber is the only writer, so every instance (including this
// one) delivers the message exactly once, via the same Redis round-trip.
func StartHub() {
	for msg := range Broadcast {
		payload, err := json.Marshal(msg)
		if err != nil {
			log.Printf("ws hub: marshal failed: %v", err)
			continue
		}
		if err := initializers.RClient.Publish(initializers.Ctx, wsBroadcastChannel, payload).Err(); err != nil {
			log.Printf("ws hub: publish failed: %v", err)
		}
	}
}

// StartSubscriber listens on the shared Redis channel and fans each message
// out to this instance's locally connected clients, filtered to the shop the
// message belongs to. Intended to be run in its own goroutine, one per
// instance, alongside StartHub.
func StartSubscriber() {
	sub := initializers.RClient.Subscribe(initializers.Ctx, wsBroadcastChannel)
	defer sub.Close()

	for redisMsg := range sub.Channel() {
		var msg Message
		if err := json.Unmarshal([]byte(redisMsg.Payload), &msg); err != nil {
			log.Printf("ws subscriber: unmarshal failed: %v", err)
			continue
		}

		if msg.ShopID == "" {
			log.Printf("ws subscriber: dropping message with empty ShopID, event=%s", msg.Event)
			continue
		}

		clientMu.Lock()
		for conn, shopID := range clients {
			if shopID != msg.ShopID {
				continue
			}
			if err := conn.WriteJSON(msg); err != nil {
				conn.Close()
				delete(clients, conn)
			}
		}
		clientMu.Unlock()
	}
}
