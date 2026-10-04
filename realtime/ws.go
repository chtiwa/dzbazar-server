package realtime

import (
	"encoding/json"
	"log"
	"sync"
	"time"

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
	clients  = make(map[*websocket.Conn]*client)
	clientMu sync.Mutex
	// Buffered so a burst of order events doesn't block senders on StartHub
	// keeping up; callers still guard the send with a timeout (see
	// controllers.processOrderEvent) in case the hub is stalled entirely.
	Broadcast = make(chan Message, 256)
)

const (
	writeWait  = 10 * time.Second
	pingPeriod = 30 * time.Second
	sendBuffer = 64
)

// client owns a single writer goroutine (writePump) — the only code that
// writes to conn. send is closed only by unregisterLocked, and only sent to
// under clientMu while still in the map, so no send-on-closed race.
type client struct {
	shopID string
	send   chan Message
}

func RegisterClient(conn *websocket.Conn, shopID string) {
	c := &client{shopID: shopID, send: make(chan Message, sendBuffer)}
	clientMu.Lock()
	clients[conn] = c
	clientMu.Unlock()
	go c.writePump(conn)
}

func UnregisterClient(conn *websocket.Conn) {
	clientMu.Lock()
	unregisterLocked(conn)
	clientMu.Unlock()
}

// unregisterLocked is idempotent: close(send) happens once, while in the map.
func unregisterLocked(conn *websocket.Conn) {
	if c, ok := clients[conn]; ok {
		delete(clients, conn)
		close(c.send)
	}
	conn.Close()
}

func (c *client) writePump(conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer UnregisterClient(conn)
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
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
		for conn, c := range clients {
			if c.shopID != msg.ShopID {
				continue
			}
			select {
			case c.send <- msg:
			default: // slow client: drop it rather than block everyone
				unregisterLocked(conn)
			}
		}
		clientMu.Unlock()
	}
}
