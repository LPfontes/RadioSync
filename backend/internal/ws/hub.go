package ws

import (
	"encoding/json"
	"log"
	"sync"
)

type Hub struct {
	StationID  string
	Clients    map[string]*Client
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex
	OnMessage  func(client *Client, msg []byte)
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.Clients)
}

func (h *Hub) HasDJConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.Clients {
		if client.Role == "dj" {
			return true
		}
	}
	return false
}

func NewHub(stationID string) *Hub {
	return &Hub{
		StationID:  stationID,
		Clients:    make(map[string]*Client),
		Broadcast:  make(chan []byte, 256),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client.ID] = client
			hasDJ := false
			for _, c := range h.Clients {
				if c.Role == "dj" {
					hasDJ = true
					break
				}
			}
			h.mu.Unlock()
			log.Printf("cliente conectado: %s (role: %s) na estação %s", client.ID, client.Role, h.StationID)
			h.BroadcastJSON(map[string]interface{}{
				"type":        "DJ_STATUS",
				"hasDJOnline": hasDJ,
			})

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client.ID]; ok {
				delete(h.Clients, client.ID)
				close(client.Send)
			}
			hasDJ := false
			for _, c := range h.Clients {
				if c.Role == "dj" {
					hasDJ = true
					break
				}
			}
			h.mu.Unlock()
			log.Printf("cliente desconectado: %s", client.ID)
			h.BroadcastJSON(map[string]interface{}{
				"type":        "DJ_STATUS",
				"hasDJOnline": hasDJ,
			})

		case message := <-h.Broadcast:
			h.mu.RLock()
			for _, client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client.ID)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) HandleMessage(client *Client, msg []byte) {
	if h.OnMessage != nil {
		h.OnMessage(client, msg)
	}
}

func (h *Hub) BroadcastJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("erro marshal broadcast: %v", err)
		return
	}
	h.Broadcast <- data
}

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
