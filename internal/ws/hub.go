package ws

import (
	"encoding/json"
	"log"
	"sync"
)

type Message struct {
	RetroID  int64
	SenderID int64
	Type     string
	Payload  interface{}
}

type Hub struct {
	rooms      map[int64]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		rooms:      make(map[int64]map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Message, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.rooms[client.RetroID] == nil {
				h.rooms[client.RetroID] = make(map[*Client]bool)
			}
			h.rooms[client.RetroID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if room, ok := h.rooms[client.RetroID]; ok {
				if _, exists := room[client]; exists {
					delete(room, client)
					close(client.Send)
					if len(room) == 0 {
						delete(h.rooms, client.RetroID)
					}
				}
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			data, err := json.Marshal(map[string]interface{}{
				"type":    msg.Type,
				"payload": msg.Payload,
			})
			if err != nil {
				log.Printf("ws broadcast marshal: %v", err)
				continue
			}

			h.mu.RLock()
			room := h.rooms[msg.RetroID]
			h.mu.RUnlock()

			for client := range room {
				if client.UserID == msg.SenderID {
					continue
				}
				select {
				case client.Send <- data:
				default:
				}
			}
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

// RoomSize returns the number of clients in a room. Used in tests.
func (h *Hub) RoomSize(retroID int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[retroID])
}

func (h *Hub) Broadcast(retroID, senderID int64, msgType string, payload interface{}) {
	h.broadcast <- Message{
		RetroID:  retroID,
		SenderID: senderID,
		Type:     msgType,
		Payload:  payload,
	}
}
