package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	StationID string      `json:"stationId"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// Client represents a WebSocket client
type Client struct {
	Conn      *websocket.Conn
	Send      chan []byte
	StationID string
	StaffID   string
	Hub       *Hub
	LastPing  time.Time
	Latency   time.Duration
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	clients    map[*Client]bool
	stations   map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mutex      sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		stations:   make(map[string]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

// Run starts the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastToAll(message)
		}
	}
}

// RegisterClient adds a new client to the hub
func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

// UnregisterClient removes a client from the hub
func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}

// BroadcastToAll sends a message to all connected clients
func (h *Hub) BroadcastToAll(message []byte) {
	h.broadcast <- message
}

// BroadcastToStation sends a message to all clients in a specific station
func (h *Hub) BroadcastToStation(stationID string, messageType string, data interface{}) {
	message := Message{
		Type:      messageType,
		StationID: stationID,
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if stationClients, ok := h.stations[stationID]; ok {
		for client := range stationClients {
			select {
			case client.Send <- jsonMessage:
			default:
				close(client.Send)
				delete(h.clients, client)
			}
		}
	}
}

// GetConnectedClients returns the number of connected clients
func (h *Hub) GetConnectedClients() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

// GetStationClients returns the number of clients connected to a specific station
func (h *Hub) GetStationClients(stationID string) int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if stationClients, ok := h.stations[stationID]; ok {
		return len(stationClients)
	}
	return 0
}

// GetClientStats returns detailed statistics about connected clients
func (h *Hub) GetClientStats() map[string]interface{} {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	stats := map[string]interface{}{
		"totalClients": len(h.clients),
		"stations":     make(map[string]interface{}),
	}

	for stationID, clients := range h.stations {
		stationStats := map[string]interface{}{
			"clientCount": len(clients),
			"clients":     make([]map[string]interface{}, 0),
		}

		for client := range clients {
			clientInfo := map[string]interface{}{
				"staffId":   client.StaffID,
				"latency":   client.Latency.Milliseconds(),
				"lastPing":  client.LastPing.Unix(),
				"connected": time.Since(client.LastPing) < 2*time.Minute,
			}
			stationStats["clients"] = append(stationStats["clients"].([]map[string]interface{}), clientInfo)
		}

		stats["stations"].(map[string]interface{})[stationID] = stationStats
	}

	return stats
}

// registerClient adds a client to the hub
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.clients[client] = true

	if h.stations[client.StationID] == nil {
		h.stations[client.StationID] = make(map[*Client]bool)
	}
	h.stations[client.StationID][client] = true

	log.Printf("Client registered for station %s. Total clients: %d",
		client.StationID, len(h.clients))
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.Send)
	}

	if stationClients, ok := h.stations[client.StationID]; ok {
		delete(stationClients, client)
		if len(stationClients) == 0 {
			delete(h.stations, client.StationID)
		}
	}

	log.Printf("Client unregistered from station %s. Total clients: %d",
		client.StationID, len(h.clients))
}

// broadcastToAll sends a message to all connected clients
func (h *Hub) broadcastToAll(message []byte) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for client := range h.clients {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			delete(h.clients, client)
		}
	}
}
