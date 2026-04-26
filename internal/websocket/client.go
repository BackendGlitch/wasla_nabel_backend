package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for now
		// In production, you should validate the origin
		return true
	},
}

// NewClient creates a new WebSocket client
func NewClient(conn *websocket.Conn, stationID, staffID string, hub *Hub) *Client {
	return &Client{
		Conn:      conn,
		Send:      make(chan []byte, 256),
		StationID: stationID,
		StaffID:   staffID,
		Hub:       hub,
		LastPing:  time.Now(),
		Latency:   0,
	}
}

// ReadPump pumps messages from the websocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.UnregisterClient(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle incoming messages
		c.handleMessage(message)
	}
}

// handleMessage processes incoming WebSocket messages
func (c *Client) handleMessage(message []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Error unmarshaling message from client %s: %v", c.StaffID, err)
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		log.Printf("Invalid message type from client %s", c.StaffID)
		return
	}

	switch msgType {
	case "ping":
		c.handlePing(msg)
	case "pong":
		c.handlePong(msg)
	case "subscribe":
		c.handleSubscribe(msg)
	case "unsubscribe":
		c.handleUnsubscribe(msg)
	default:
		log.Printf("Unknown message type from client %s: %s", c.StaffID, msgType)
	}
}

// handlePing responds to ping messages with pong
func (c *Client) handlePing(msg map[string]interface{}) {
	timestamp, ok := msg["timestamp"].(float64)
	if !ok {
		timestamp = float64(time.Now().UnixNano() / int64(time.Millisecond))
	}

	pongMsg := map[string]interface{}{
		"type":       "pong",
		"timestamp":  timestamp,
		"serverTime": time.Now().UnixNano() / int64(time.Millisecond),
	}

	response, err := json.Marshal(pongMsg)
	if err != nil {
		log.Printf("Error marshaling pong response: %v", err)
		return
	}

	select {
	case c.Send <- response:
	default:
		log.Printf("Failed to send pong to client %s", c.StaffID)
	}
}

// handlePong processes pong responses for latency calculation
func (c *Client) handlePong(msg map[string]interface{}) {
	clientTime, ok := msg["timestamp"].(float64)
	if !ok {
		return
	}

	serverTime := time.Now().UnixNano() / int64(time.Millisecond)
	latency := time.Duration(serverTime-int64(clientTime)) * time.Millisecond
	c.Latency = latency
	c.LastPing = time.Now()

	log.Printf("Client %s latency: %v", c.StaffID, latency)
}

// handleSubscribe processes subscription requests
func (c *Client) handleSubscribe(msg map[string]interface{}) {
	events, ok := msg["events"].([]interface{})
	if !ok {
		log.Printf("Invalid subscribe message from client %s", c.StaffID)
		return
	}

	log.Printf("Client %s subscribed to events: %v", c.StaffID, events)

	// Send subscription confirmation
	response := map[string]interface{}{
		"type":      "subscription_confirmed",
		"events":    events,
		"timestamp": time.Now().UnixNano() / int64(time.Millisecond),
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling subscription confirmation: %v", err)
		return
	}

	select {
	case c.Send <- responseBytes:
	default:
		log.Printf("Failed to send subscription confirmation to client %s", c.StaffID)
	}
}

// handleUnsubscribe processes unsubscription requests
func (c *Client) handleUnsubscribe(msg map[string]interface{}) {
	events, ok := msg["events"].([]interface{})
	if !ok {
		log.Printf("Invalid unsubscribe message from client %s", c.StaffID)
		return
	}

	log.Printf("Client %s unsubscribed from events: %v", c.StaffID, events)
}

// WritePump pumps messages from the hub to the websocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWS handles websocket requests from clients
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request, stationID, staffID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	client := NewClient(conn, stationID, staffID, hub)
	hub.RegisterClient(client)

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines
	go client.WritePump()
	go client.ReadPump()
}
