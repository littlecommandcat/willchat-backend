package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
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
var errUserNotFound = errors.New("user not found")

type groupRecord struct {
	GroupID   string `json:"group_id"`
	ID        string `json:"id,omitempty"` // legacy Redis records
	Name      string `json:"name"`
	OwnerID   string `json:"owner_id"`
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
	hub       *core.Hub

	groupMu sync.Mutex
}

func NewChatService(redisClient *redis.Client, snowflake *core.Snowflake, authService *Service, hub *core.Hub) *ChatService {
	return &ChatService{
		redis:     redisClient,
		snowflake: snowflake,
		auth:      authService,
		hub:       hub,
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
	canCreate, err := s.auth.canCreateGroup(userID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !canCreate {
		writeError(c, http.StatusConflict, "group_limit_reached", "a user can create at most 15 groups")
		return
	}

	group := groupRecord{
		GroupID:   s.snowflake.Next(),
		Name:      name,
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
		_ = s.redis.Del(context.Background(), groupKey(group.GroupID), groupMembersKey(group.GroupID), userGroupsKey(userID), ownedGroupsKey(userID)).Err()
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
	s.hub.Broadcast(groupID, core.WSEvent{
		Event:   "member_join",
		GroupID: groupID,
		Data: gin.H{
			"group_id": groupID,
			"user_id":  userID,
		},
	})
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

	found, err := s.redis.SIsMember(context.Background(), groupMembersKey(groupID), userID).Result()
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !found {
		writeError(c, http.StatusNotFound, "not_group_member", "user is not a member of this group")
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
	s.hub.Broadcast(groupID, core.WSEvent{
		Event:   "member_leave",
		GroupID: groupID,
		Data: gin.H{
			"group_id": groupID,
			"user_id":  userID,
		},
	})

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

	message, _, err := s.createMessage(groupID, userID, content)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			writeError(c, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	s.hub.Broadcast(groupID, core.WSEvent{Event: "message", GroupID: groupID, Data: message})
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: message})
}

func (s *ChatService) WebSocket(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		if headerToken, ok := bearerToken(c); ok {
			token = headerToken
		}
	}
	userID, ok := s.auth.validateToken(token)
	if !ok {
		code := "invalid_token"
		message := "token is invalid"
		if token == "" {
			code = "missing_token"
			message = "a bearer token is required"
		}
		writeError(c, http.StatusUnauthorized, code, message)
		return
	}

	websocket.Server{
		Handler: websocket.Handler(func(conn *websocket.Conn) {
			client := s.hub.Register(userID, "")
			defer s.hub.Unregister(client)
			defer conn.Close()

			go func() {
				for payload := range client.Send {
					if err := websocket.Message.Send(conn, string(payload)); err != nil {
						_ = conn.Close()
						return
					}
				}
				_ = conn.Close()
			}()

			for {
				var incoming any
				if err := websocket.JSON.Receive(conn, &incoming); err != nil {
					return
				}
			}
		}),
		Handshake: func(*websocket.Config, *http.Request) error {
			return nil
		},
	}.ServeHTTP(c.Writer, c.Request)
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
	return group, nil
}

func (s *ChatService) addMember(groupID, userID string) error {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	ctx := context.Background()
	member, err := s.redis.SIsMember(ctx, groupMembersKey(groupID), userID).Result()
	if err != nil {
		return err
	}
	if member {
		return errAlreadyGroupMember
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

func (s *ChatService) createMessage(groupID, userID, content string) (core.Message, []byte, error) {
	authorID, err := strconv.Atoi(userID)
	if err != nil {
		return core.Message{}, nil, err
	}
	message := core.Message{
		Id:       s.snowflake.Next(),
		AuthorId: authorID,
		Content:  content,
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return core.Message{}, nil, err
	}
	if err := s.redis.RPush(context.Background(), messagesKey(groupID), payload).Err(); err != nil {
		log.Printf("redis message write failed group_id=%s message_id=%s: %v", groupID, message.Id, err)
		return core.Message{}, nil, err
	}
	log.Printf("redis message stored group_id=%s message_id=%s user_id=%s", groupID, message.Id, userID)
	return message, payload, nil
}

func validateMessageContent(value string) (string, error) {
	content := strings.TrimSpace(value)
	if content == "" || len([]rune(content)) > maxMessageLength {
		return "", errors.New("invalid message content")
	}
	return content, nil
}

func currentUserID(c *gin.Context) string {
	userID, _ := c.Get("user_id")
	return userID.(string)
}

func (s *Service) userByID(userID string) (core.UserData, error) {
	value, err := s.redis.Get(context.Background(), userKey(userID)).Result()
	if err != nil {
		return core.UserData{}, err
	}
	var record redisUserRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return core.UserData{}, err
	}
	if record.ID == "" {
		return core.UserData{}, errors.New("invalid redis user record")
	}
	return core.UserData{Id: record.ID, Username: record.Username, Email: normalizeEmail(record.Email), PasswordHash: record.PasswordHash, Token: record.Token}, nil
}

func (s *Service) userByEmail(email string) (core.UserData, error) {
	ctx := context.Background()
	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "user:*", 0).Result()
		if err != nil {
			return core.UserData{}, err
		}
		for _, key := range keys {
			if !isUserRecordKey(key) {
				continue
			}
			userID := strings.TrimPrefix(key, "user:")
			user, err := s.userByID(userID)
			if err != nil {
				return core.UserData{}, err
			}
			if user.Email == normalizeEmail(email) {
				return user, nil
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return core.UserData{}, redis.Nil
		}
	}
}

func isUserRecordKey(key string) bool {
	return strings.HasPrefix(key, "user:") && strings.Count(key, ":") == 1 && strings.TrimPrefix(key, "user:") != ""
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
func ownedGroupsKey(userID string) string   { return "user:" + userID + ":owned-groups" }
func messagesKey(groupID string) string     { return "group:" + groupID + ":messages" }
