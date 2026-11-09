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
	memberResponse func(string) (any, error)
	mu             sync.Mutex
	clients        map[*Client]struct{}
	members        map[string]map[*Client]struct{}
	closed         map[*Client]struct{}
}

// SetMemberResponse configures the public member representation used by presence events.
func (h *Hub) SetMemberResponse(resolve func(string) (any, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.memberResponse = resolve
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
		defer h.presenceLocked(memberID, groupID, WSEventEnum.MemberOnline)
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
	if len(h.members[memberID]) > 0 {
		h.presenceLocked(memberID, []string{groupID}, WSEventEnum.MemberOnline)
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
		log.Printf("marshal websocket event failed: %v", err)
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
	close(client.Send)
	deleteConnection()
	if len(h.members[client.MemberID]) == 0 {
		delete(h.members, client.MemberID)
		h.presenceLocked(client.MemberID, client.GroupID, WSEventEnum.MemberOffline)
	}
}

// presenceLocked tells everyone sharing a group with memberID that they went online/offline.
func (h *Hub) presenceLocked(memberID string, groups []string, event wsEventType) {
	var member any = map[string]string{"id": memberPrefix + memberID}
	if h.memberResponse != nil {
		var err error
		member, err = h.memberResponse(memberID)
		if err != nil {
			log.Printf("presence member lookup failed member_id=%s: %v", memberID, err)
			return
		}
	}
	payload, err := json.Marshal(WSEvent{Event: event, Payload: map[string]any{"member": member}})
	if err != nil {
		return
	}
	for client := range h.clients {
		if client.MemberID == memberID || !sharesGroup(client.GroupID, groups) {
			continue
		}
		select {
		case client.Send <- payload:
		default:
			h.closeLocked(client)
		}
	}
}

// OnlineFor lists online members (external IDs) sharing a group with memberID.
func (h *Hub) OnlineFor(memberID string, groups []string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	seen := map[string]struct{}{}
	online := []string{}
	for client := range h.clients {
		if _, ok := seen[client.MemberID]; ok || client.MemberID == memberID || !sharesGroup(client.GroupID, groups) {
			continue
		}
		seen[client.MemberID] = struct{}{}
		online = append(online, memberPrefix+client.MemberID)
	}
	return online
}

func sharesGroup(a, b []string) bool {
	for _, id := range a {
		if containsGroup(b, id) {
			return true
		}
	}
	return false
}

func containsGroup(groups []string, groupID string) bool {
	for _, id := range groups {
		if id == groupID {
			return true
		}
	}
	return false
}
