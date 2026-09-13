package core

import (
	"encoding/json"
	"sync"
)

type WSEvent struct {
	Event   string `json:"event"`
	Data    any    `json:"data,omitempty"`
	Content string `json:"content,omitempty"`
}

type WSMessage struct {
	Content string `json:"content"`
}

type Client struct {
	UserID  string
	GroupID string
	Send    chan []byte
}

type Hub struct {
	mu     sync.Mutex
	groups map[string]map[*Client]struct{}
	users  map[string]map[*Client]struct{}
	closed map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		groups: make(map[string]map[*Client]struct{}),
		users:  make(map[string]map[*Client]struct{}),
		closed: make(map[*Client]struct{}),
	}
}

func (h *Hub) Register(userID, groupID string) *Client {
	client := &Client{UserID: userID, GroupID: groupID, Send: make(chan []byte, 16)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.groups[groupID] == nil {
		h.groups[groupID] = make(map[*Client]struct{})
	}
	if h.users[userID] == nil {
		h.users[userID] = make(map[*Client]struct{})
	}
	h.groups[groupID][client] = struct{}{}
	h.users[userID][client] = struct{}{}
	return client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closeLocked(client)
}

func (h *Hub) CloseUser(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.users[userID] {
		h.closeLocked(client)
	}
}

func (h *Hub) Broadcast(groupID string, event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.groups[groupID] {
		select {
		case client.Send <- payload:
		default:
			h.closeLocked(client)
		}
	}
}

func (h *Hub) closeLocked(client *Client) {
	if _, ok := h.closed[client]; ok {
		return
	}
	h.closed[client] = struct{}{}
	delete(h.groups[client.GroupID], client)
	if len(h.groups[client.GroupID]) == 0 {
		delete(h.groups, client.GroupID)
	}
	delete(h.users[client.UserID], client)
	if len(h.users[client.UserID]) == 0 {
		delete(h.users, client.UserID)
	}
	close(client.Send)
}
