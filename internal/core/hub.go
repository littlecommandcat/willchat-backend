package core

import (
	"encoding/json"
	"log"
	"sync"
)

var (
	ConnectionCount int = 0
)

type Client struct {
	MemberID string
	GroupID  []string
	Send     chan []byte
}

type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	members map[string]map[*Client]struct{}
	closed  map[*Client]struct{}
}

func deleteConnection() {
	ConnectionCount -= 1
	if ConnectionCount < 0 {
		ConnectionCount = 0
	}
}

func addConnection() {
	ConnectionCount += 1
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		members: make(map[string]map[*Client]struct{}),
		closed:  make(map[*Client]struct{}),
	}
}

func (h *Hub) Register(memberID string, groupID []string) *Client {
	client := &Client{MemberID: memberID, GroupID: groupID, Send: make(chan []byte, 16)}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
	if h.members[memberID] == nil {
		h.members[memberID] = make(map[*Client]struct{})
	}
	h.members[memberID][client] = struct{}{}
	log.Printf("Register ws as %s, %s", memberID, groupID)
	addConnection()
	return client
}

func (h *Hub) AddGroup(memberID, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.members[memberID] {
		if containsGroup(client.GroupID, groupID) {
			continue
		}

		client.GroupID = append(client.GroupID, groupID)
	}
}

func (h *Hub) RemoveGroup(memberID, groupID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.members[memberID] {
		for i, id := range client.GroupID {
			if id != groupID {
				continue
			}

			client.GroupID = append(
				client.GroupID[:i],
				client.GroupID[i+1:]...,
			)
			break
		}
	}
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

func (h *Hub) BroadcastOnlyMember(memberID string, event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Fatalf("Error: %s", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.members[memberID] {
		select {
		case client.Send <- payload:
		default:
			h.closeLocked(client)
		}
	}
}

func (h *Hub) BroadcastOnlyGroup(groupID string, event WSEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("marshal websocket event failed: %v", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for client := range h.clients {
		if !containsGroup(client.GroupID, groupID) {
			log.Printf("Not equal %s and %s", client.GroupID, groupID)
			continue
		}

		select {
		case client.Send <- payload:
		default:
			h.closeLocked(client)
		}
	}
}

// func (h *Hub) Broadcast(groupID string, event WSEvent) {
// 	h.BroadcastAll(event)
// }

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
	deleteConnection()
}

func containsGroup(groups []string, groupID string) bool {
	for _, id := range groups {
		if id == groupID {
			return true
		}
	}
	return false
}
