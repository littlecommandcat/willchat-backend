package core

import (
	"encoding/json"
	"sync"
)

type Client struct {
	MemberID string
	GroupID  string
	Send     chan []byte
}

type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	members map[string]map[*Client]struct{}
	closed  map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		members: make(map[string]map[*Client]struct{}),
		closed:  make(map[*Client]struct{}),
	}
}

func (h *Hub) Register(memberID, groupID string) *Client {
	client := &Client{MemberID: memberID, GroupID: groupID, Send: make(chan []byte, 16)}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
	if h.members[memberID] == nil {
		h.members[memberID] = make(map[*Client]struct{})
	}
	h.members[memberID][client] = struct{}{}
	return client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closeLocked(client)
}

func (h *Hub) CloseMember(memberID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.members[memberID] {
		h.closeLocked(client)
	}
}

func (h *Hub) Broadcast(groupID string, event WSEvent) {
	h.BroadcastAll(event)
}

func (h *Hub) BroadcastAll(event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
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
	delete(h.clients, client)
	delete(h.members[client.MemberID], client)
	if len(h.members[client.MemberID]) == 0 {
		delete(h.members, client.MemberID)
	}
	close(client.Send)
}
