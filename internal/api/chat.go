package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/net/websocket"
)

const (
	maxGroupNameLength = 100
	maxMessageLength   = 4000
	maxOwnedGroups     = 15
)

var errGroupLimitReached = errors.New("group limit reached")
var errAlreadyGroupMember = errors.New("already a group member")

type groupRecord struct {
	GroupID   string   `json:"group_id"`
	ID        string   `json:"id,omitempty"` // legacy Redis records
	Name      string   `json:"name"`
	Users     []string `json:"users"`
	OwnerID   string   `json:"owner_id"`
	CreatedAt string   `json:"created_at"`
}

type chatMessage struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type createGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

type sendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type ChatService struct {
	redis     *redis.Client
	snowflake *core.Snowflake
	auth      *Service

	groupMu       sync.Mutex
	connectionsMu sync.Mutex
	connections   map[string]map[*websocket.Conn]struct{}
}

func NewChatService(redisClient *redis.Client, snowflake *core.Snowflake, authService *Service) *ChatService {
	return &ChatService{
		redis:       redisClient,
		snowflake:   snowflake,
		auth:        authService,
		connections: make(map[string]map[*websocket.Conn]struct{}),
	}
}

func (s *ChatService) CreateGroup(c *gin.Context) {
	userID := currentUserID(c)
	var request createGroupRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len([]rune(name)) > maxGroupNameLength {
		writeError(c, http.StatusBadRequest, "invalid_group_name", "group name must be between 1 and 100 characters")
		return
	}
	if !s.auth.canCreateGroup(userID) {
		writeError(c, http.StatusConflict, "group_limit_reached", "a user can create at most 15 groups")
		return
	}

	group := groupRecord{
		GroupID:   s.snowflake.Next(),
		Name:      name,
		Users:     []string{},
		OwnerID:   userID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.storeGroup(group); err != nil {
		log.Printf("redis group write failed group_id=%s: %v", group.GroupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.addMember(group.GroupID, userID); err != nil {
		_ = s.redis.Del(context.Background(), groupKey(group.GroupID)).Err()
		log.Printf("redis group owner membership write failed group_id=%s user_id=%s: %v", group.GroupID, userID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.auth.addOwnedGroup(userID, group.GroupID); err != nil {
		_ = s.redis.Del(context.Background(), groupKey(group.GroupID), groupMembersKey(group.GroupID), userGroupsKey(userID)).Err()
		log.Printf("redis user group write failed group_id=%s user_id=%s: %v", group.GroupID, userID, err)
		if errors.Is(err, errGroupLimitReached) {
			writeError(c, http.StatusConflict, "group_limit_reached", "a user can create at most 15 groups")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: group})
}

func (s *ChatService) GetGroup(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	group, err := s.getGroup(groupID)
	if errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	}
	if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if group.GroupID == "" {
		group.GroupID = groupID
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: group})
}

func (s *ChatService) JoinGroup(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	userID := currentUserID(c)
	if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if s.isMember(groupID, userID) {
		writeError(c, http.StatusConflict, "already_group_member", "user is already a member of this group")
		return
	}

	if err := s.addMember(groupID, userID); err != nil {
		if errors.Is(err, errAlreadyGroupMember) {
			writeError(c, http.StatusConflict, "already_group_member", "user is already a member of this group")
			return
		}
		log.Printf("redis group member write failed group_id=%s user_id=%s: %v", groupID, userID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id": groupID,
		"user_id":  userID,
	}})
}

func (s *ChatService) LeaveGroup(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	userID := currentUserID(c)
	group, err := s.getGroup(groupID)
	if errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	}
	if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if group.OwnerID == userID {
		writeError(c, http.StatusConflict, "owner_cannot_leave", "group owner cannot leave the group")
		return
	}

	found := false
	updatedUsers := make([]string, 0, len(group.Users))
	for _, memberID := range group.Users {
		if memberID == userID {
			found = true
			continue
		}
		updatedUsers = append(updatedUsers, memberID)
	}
	if !found {
		writeError(c, http.StatusNotFound, "not_group_member", "user is not a member of this group")
		return
	}

	group.Users = updatedUsers
	if err := s.storeGroup(group); err != nil {
		log.Printf("redis group member removal failed group_id=%s user_id=%s: %v", groupID, userID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	ctx := context.Background()
	if err := s.redis.SRem(ctx, groupMembersKey(groupID), userID).Err(); err != nil {
		log.Printf("redis group member set removal failed group_id=%s user_id=%s: %v", groupID, userID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.redis.SRem(ctx, userGroupsKey(userID), groupID).Err(); err != nil {
		log.Printf("redis user group removal failed group_id=%s user_id=%s: %v", groupID, userID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id": groupID,
		"user_id":  userID,
	}})
}

func (s *ChatService) SendMessage(c *gin.Context) {
	groupID := strings.TrimSpace(c.Param("group_id"))
	userID := currentUserID(c)
	var request sendMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "content is required")
		return
	}
	content := strings.TrimSpace(request.Content)
	if content == "" || len([]rune(content)) > maxMessageLength {
		writeError(c, http.StatusBadRequest, "invalid_message", "message content must be between 1 and 4000 characters")
		return
	}

	if !s.isMember(groupID, userID) {
		if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
			writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		} else {
			writeError(c, http.StatusForbidden, "not_group_member", "user is not a member of this group")
		}
		return
	}

	user, ok := s.auth.userByID(userID)
	if !ok {
		writeError(c, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	message := chatMessage{
		ID:        s.snowflake.Next(),
		GroupID:   groupID,
		UserID:    userID,
		Email:     user.Email,
		Content:   content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(message)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "message_encode_failed", "unable to encode message")
		return
	}
	ctx := context.Background()
	if err := s.redis.RPush(ctx, messagesKey(groupID), payload).Err(); err != nil {
		log.Printf("redis message write failed group_id=%s message_id=%s: %v", groupID, message.ID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	log.Printf("redis message stored group_id=%s message_id=%s user_id=%s", groupID, message.ID, userID)
	s.broadcast(groupID, payload)
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: message})
}

func (s *ChatService) WebSocket(c *gin.Context) {
	token, ok := websocketToken(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, core.Response{Status: "error", Code: "missing_token", Data: gin.H{"message": "a bearer token or token query parameter is required"}})
		return
	}
	userID, ok := s.auth.validateToken(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, core.Response{Status: "error", Code: "invalid_token", Data: gin.H{"message": "token is invalid"}})
		return
	}
	groupID := strings.TrimSpace(c.Param("group_id"))
	if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
		c.JSON(http.StatusNotFound, core.Response{Status: "error", Code: "group_not_found", Data: gin.H{"message": "group not found"}})
		return
	} else if err != nil {
		log.Printf("redis websocket group read failed group_id=%s: %v", groupID, err)
		c.JSON(http.StatusServiceUnavailable, core.Response{Status: "error", Code: "redis_unavailable", Data: gin.H{"message": errRedisUnavailable.Error()}})
		return
	}
	if !s.isMember(groupID, userID) {
		c.JSON(http.StatusForbidden, core.Response{Status: "error", Code: "not_group_member", Data: gin.H{"message": "user is not a member of this group"}})
		return
	}

	websocket.Handler(func(conn *websocket.Conn) {
		s.addConnection(groupID, conn)
		defer s.removeConnection(groupID, conn)
		defer conn.Close()
		for {
			var incoming string
			if err := websocket.Message.Receive(conn, &incoming); err != nil {
				return
			}
		}
	}).ServeHTTP(c.Writer, c.Request)
}

func (s *ChatService) storeGroup(group groupRecord) error {
	if group.GroupID == "" {
		group.GroupID = group.ID
	}
	payload, err := json.Marshal(group)
	if err != nil {
		return err
	}
	key := groupKey(group.GroupID)
	if err := s.redis.Set(context.Background(), key, payload, 0).Err(); err != nil {
		return err
	}
	stored, err := s.redis.Get(context.Background(), key).Result()
	if err != nil {
		return err
	}
	if stored != string(payload) {
		return errors.New("redis group verification failed")
	}
	return nil
}

func (s *ChatService) getGroup(groupID string) (groupRecord, error) {
	value, err := s.redis.Get(context.Background(), groupKey(groupID)).Result()
	if err != nil {
		return groupRecord{}, err
	}
	var group groupRecord
	if err := json.Unmarshal([]byte(value), &group); err != nil {
		return groupRecord{}, err
	}
	if group.GroupID == "" {
		group.GroupID = group.ID
	}
	if group.Users == nil {
		members, membersErr := s.redis.SMembers(context.Background(), groupMembersKey(group.GroupID)).Result()
		if membersErr == nil {
			group.Users = members
		}
	}
	return group, nil
}

func (s *ChatService) addMember(groupID, userID string) error {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	ctx := context.Background()
	group, err := s.getGroup(groupID)
	if err != nil {
		return err
	}
	for _, memberID := range group.Users {
		if memberID == userID {
			if err := s.redis.SAdd(ctx, groupMembersKey(groupID), userID).Err(); err != nil {
				return err
			}
			return errAlreadyGroupMember
		}
	}
	group.Users = append(group.Users, userID)
	if err := s.storeGroup(group); err != nil {
		return err
	}
	if err := s.redis.SAdd(ctx, groupMembersKey(groupID), userID).Err(); err != nil {
		return err
	}
	return s.redis.SAdd(ctx, userGroupsKey(userID), groupID).Err()
}

func (s *ChatService) isMember(groupID, userID string) bool {
	member, err := s.redis.SIsMember(context.Background(), groupMembersKey(groupID), userID).Result()
	return err == nil && member
}

func (s *ChatService) addConnection(groupID string, conn *websocket.Conn) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if s.connections[groupID] == nil {
		s.connections[groupID] = make(map[*websocket.Conn]struct{})
	}
	s.connections[groupID][conn] = struct{}{}
	log.Printf("websocket connected group_id=%s connections=%d", groupID, len(s.connections[groupID]))
}

func (s *ChatService) removeConnection(groupID string, conn *websocket.Conn) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if groupConnections := s.connections[groupID]; groupConnections != nil {
		delete(groupConnections, conn)
		if len(groupConnections) == 0 {
			delete(s.connections, groupID)
		}
	}
	log.Printf("websocket disconnected group_id=%s", groupID)
}

func (s *ChatService) broadcast(groupID string, payload []byte) {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	for conn := range s.connections[groupID] {
		if err := websocket.Message.Send(conn, string(payload)); err != nil {
			log.Printf("websocket broadcast failed group_id=%s: %v", groupID, err)
			_ = conn.Close()
			delete(s.connections[groupID], conn)
		}
	}
}

func currentUserID(c *gin.Context) string {
	userID, _ := c.Get("user_id")
	return userID.(string)
}

func websocketToken(c *gin.Context) (string, bool) {
	if token, ok := bearerToken(c); ok {
		return token, true
	}
	token := strings.TrimSpace(c.Query("token"))
	return token, token != ""
}

func (s *Service) userByID(userID string) (core.UserData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Id == userID {
			return user, true
		}
	}
	return core.UserData{}, false
}

func (s *Service) validateToken(token string) (string, bool) {
	value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
	if err != nil {
		return "", false
	}
	var current tokenRecord
	if json.Unmarshal([]byte(value), &current) != nil || current.UserID == "" {
		return "", false
	}
	return current.UserID, true
}

func groupKey(groupID string) string        { return "group:" + groupID }
func groupMembersKey(groupID string) string { return "group:" + groupID + ":members" }
func userGroupsKey(userID string) string    { return "user:" + userID + ":groups" }
func messagesKey(groupID string) string     { return "group:" + groupID + ":messages" }
