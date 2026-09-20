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

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	maxGroupNameLength = 100
	maxMessageLength   = 4000
	maxGetAmount       = 50
	maxOwnedGroups     = 15
)

var errGroupLimitReached = errors.New("group limit reached")
var errAlreadyGroupMember = errors.New("already a group member")

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
	memberID := currentMemberID(c)
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
	canCreate, err := s.auth.canCreateGroup(memberID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !canCreate {
		writeError(c, http.StatusConflict, "group_limit_reached", "a member can create at most 15 groups")
		return
	}

	group := groupRecord{
		GroupID:   s.snowflake.Next(),
		Name:      name,
		OwnerID:   memberID,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.storeGroup(group); err != nil {
		log.Printf("redis group write failed group_id=%s: %v", group.GroupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.addMember(group.GroupID, memberID); err != nil {
		_ = s.redis.Del(context.Background(), groupKey(group.GroupID)).Err()
		log.Printf("redis group owner membership write failed group_id=%s member_id=%s: %v", group.GroupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.auth.addOwnedGroup(memberID, group.GroupID); err != nil {
		ctx := context.Background()
		_ = s.redis.Del(ctx, groupKey(group.GroupID), groupMembersKey(group.GroupID)).Err()
		_ = s.redis.SRem(ctx, memberGroupsKey(memberID), group.GroupID).Err()
		_ = s.redis.SRem(ctx, ownedGroupsKey(memberID), group.GroupID).Err()
		log.Printf("redis member group write failed group_id=%s member_id=%s: %v", group.GroupID, memberID, err)
		if errors.Is(err, errGroupLimitReached) {
			writeError(c, http.StatusConflict, "group_limit_reached", "a member can create at most 15 groups")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": toGroupResponse(s.snowflake, group)}})
}

func (s *ChatService) GetGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
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
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": toGroupResponse(s.snowflake, group)}})
}

func (s *ChatService) JoinGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)
	if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if s.isMember(groupID, memberID) {
		writeError(c, http.StatusConflict, "already_group_member", "member is already a member of this group")
		return
	}

	if err := s.addMember(groupID, memberID); err != nil {
		if errors.Is(err, errAlreadyGroupMember) {
			writeError(c, http.StatusConflict, "already_group_member", "member is already a member of this group")
			return
		}
		log.Printf("redis group member write failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	responseGroupID := s.snowflake.ToGroupID(groupID)
	responseMemberID := s.snowflake.ToMemberID(memberID)
	s.hub.Broadcast(groupID, core.WSEvent{
		Event:   core.WSEventEnum.JoinGroup,
		GroupID: responseGroupID,
		Data: gin.H{
			"group_id":  responseGroupID,
			"member_id": responseMemberID,
		},
	})
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id":  responseGroupID,
		"member_id": responseMemberID,
	}})
}

func (s *ChatService) LeaveGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)
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
	if group.OwnerID == memberID {
		writeError(c, http.StatusConflict, "owner_cannot_leave", "group owner cannot leave the group")
		return
	}

	found, err := s.redis.SIsMember(context.Background(), groupMembersKey(groupID), memberID).Result()
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !found {
		writeError(c, http.StatusNotFound, "not_group_member", "member is not a member of this group")
		return
	}

	ctx := context.Background()
	if err := s.redis.SRem(ctx, groupMembersKey(groupID), memberID).Err(); err != nil {
		log.Printf("redis group member set removal failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.redis.SRem(ctx, memberGroupsKey(memberID), groupID).Err(); err != nil {
		log.Printf("redis member group removal failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	responseGroupID := s.snowflake.ToGroupID(groupID)
	responseMemberID := s.snowflake.ToMemberID(memberID)
	s.hub.Broadcast(groupID, core.WSEvent{
		Event:   core.WSEventEnum.LeaveGroup,
		GroupID: responseGroupID,
		Data: gin.H{
			"group_id":  responseGroupID,
			"member_id": responseMemberID,
		},
	})

	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id":  responseGroupID,
		"member_id": responseMemberID,
	}})
}

func (s *ChatService) DeleteGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)

	s.groupMu.Lock()
	defer s.groupMu.Unlock()

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
	if group.OwnerID != memberID {
		writeError(c, http.StatusForbidden, "not_group_owner", "only the group owner can delete this group")
		return
	}

	ctx := context.Background()
	members, err := s.redis.SMembers(ctx, groupMembersKey(groupID)).Result()
	if err != nil {
		log.Printf("redis group member read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	if _, err := s.redis.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, groupMemberID := range members {
			pipe.SRem(ctx, memberGroupsKey(groupMemberID), groupID)
		}
		pipe.SRem(ctx, ownedGroupsKey(memberID), groupID)
		pipe.Del(ctx, groupKey(groupID), groupMembersKey(groupID), messagesKey(groupID))
		return nil
	}); err != nil {
		log.Printf("redis group deletion failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	responseGroupID := s.snowflake.ToGroupID(groupID)
	s.hub.Broadcast(groupID, core.WSEvent{
		Event:   core.WSEventEnum.DeleteGroup,
		GroupID: responseGroupID,
		Data: gin.H{
			"group_id": responseGroupID,
		},
	})
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id": responseGroupID,
	}})
}

func (s *ChatService) SendMessage(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)
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

	if !s.isMember(groupID, memberID) {
		if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
			writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		} else {
			writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		}
		return
	}

	message, _, err := s.createMessage(groupID, memberID, content)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	responseMessage := toMessageResponse(s.snowflake, message)
	s.hub.Broadcast(groupID, core.WSEvent{Event: core.WSEventEnum.SendMessage, GroupID: s.snowflake.ToGroupID(groupID), Data: responseMessage})
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{"message": responseMessage}})
}

func (s *ChatService) GetMessages(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)
	var request getMessageRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "amount is required")
		return
	}
	amount := request.Amount
	if amount <= 0 || amount > maxGetAmount {
		writeError(c, http.StatusBadRequest, "invalid_request", "message amount must be between 1 and 50")
		return
	}

	if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !s.isMember(groupID, memberID) {
		writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		return
	}

	messages, err := s.getMessages(groupID, amount)
	if err != nil {
		log.Printf("redis messages read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	messageResponses := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		messageResponses = append(messageResponses, toMessageResponse(s.snowflake, message))
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"messages": messageResponses, "amount": amount}})
}

func (s *ChatService) GetMember(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(c.Param("member_id"))
	requesterID := currentMemberID(c)

	if groupID == "" || memberID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "group_id and member_id are required")
		return
	}

	if _, err := s.getGroup(groupID); errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("redis group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !s.isMember(groupID, requesterID) {
		writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		return
	}
	if !s.isMember(groupID, memberID) {
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}

	member, err := s.auth.memberByID(memberID)
	if errors.Is(err, redis.Nil) {
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}
	if err != nil {
		log.Printf("redis member read failed member_id=%s: %v", memberID, err)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group_id": s.snowflake.ToGroupID(groupID),
		"member":   memberProfile{ID: s.snowflake.ToMemberID(member.Id), Username: member.Username, CreatedAt: s.snowflake.ToTime(member.Id)},
	}})
}

// toGroupResponse converts an internal group record (raw snowflake IDs) into
// its external representation with the mr-/gr- prefixes.
func toGroupResponse(snowflake *core.Snowflake, group groupRecord) groupResponse {
	response := groupResponse{Name: group.Name, CreatedAt: group.CreatedAt}
	if group.GroupID != "" {
		response.GroupID = snowflake.ToGroupID(group.GroupID)
	}
	if group.OwnerID != "" {
		response.OwnerID = snowflake.ToMemberID(group.OwnerID)
	}
	return response
}

// toMessageResponse converts an internal message (raw snowflake IDs) into its
// external representation, prefixing the member ID of the author.
func toMessageResponse(snowflake *core.Snowflake, message core.Message) messageResponse {
	response := messageResponse{
		Content:   message.Content,
		Id:        message.Id,
		Timestamp: message.Timestamp,
		Type:      message.Type,
	}
	if message.AuthorId != "" {
		response.AuthorId = snowflake.ToMemberID(message.AuthorId)
	}
	return response
}

func (s *ChatService) WebSocket(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		if headerToken, ok := bearerToken(c); ok {
			token = headerToken
		}
	}
	memberID, ok := s.auth.validateToken(token)
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

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("WebSocket accept failed: %v", err)
		return
	}
	defer conn.CloseNow()

	client := s.hub.Register(memberID, "")
	log.Printf("Websocket connected: %s", memberID)
	defer s.hub.Unregister(client)

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	go func() {
		for payload := range client.Send {
			if err := wsjson.Write(ctx, conn, json.RawMessage(payload)); err != nil {
				cancel()
				return
			}
		}
		cancel()
	}()

	for {
		var incoming any
		if err := wsjson.Read(ctx, conn, &incoming); err != nil {
			cancel()
			return
		}
	}
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

func (s *ChatService) addMember(groupID, memberID string) error {
	s.groupMu.Lock()
	defer s.groupMu.Unlock()

	ctx := context.Background()
	isAlreadyMember, err := s.redis.SIsMember(ctx, groupMembersKey(groupID), memberID).Result()
	if err != nil {
		return err
	}
	if isAlreadyMember {
		return errAlreadyGroupMember
	}
	if err := s.redis.SAdd(ctx, groupMembersKey(groupID), memberID).Err(); err != nil {
		return err
	}
	return s.redis.SAdd(ctx, memberGroupsKey(memberID), groupID).Err()
}

func (s *ChatService) isMember(groupID, memberID string) bool {
	isGroupMember, err := s.redis.SIsMember(context.Background(), groupMembersKey(groupID), memberID).Result()
	return err == nil && isGroupMember
}

func (s *ChatService) createMessage(groupID, memberID, content string) (core.Message, []byte, error) {
	id := s.snowflake.Next()
	// timestamp :=

	// Set msgType for test
	message := core.Message{
		Id:        id,
		AuthorId:  memberID,
		Timestamp: s.snowflake.ToTime(id),
		Content:   content,
		Type:      core.MessageTypeEnum.Normal,
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return core.Message{}, nil, err
	}
	if err := s.redis.RPush(context.Background(), messagesKey(groupID), payload).Err(); err != nil {
		log.Printf("redis message write failed group_id=%s message_id=%s: %v", groupID, message.Id, err)
		return core.Message{}, nil, err
	}
	log.Printf("redis message stored group_id=%s message_id=%s member_id=%s", groupID, message.Id, memberID)
	return message, payload, nil
}

func (s *ChatService) getMessages(groupID string, amount int) ([]core.Message, error) {
	values, err := s.redis.LRange(context.Background(), messagesKey(groupID), -int64(amount), -1).Result()
	if err != nil {
		return nil, err
	}

	messages := make([]core.Message, 0, len(values))
	for _, value := range values {
		var message core.Message
		if err := json.Unmarshal([]byte(value), &message); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func validateMessageContent(value string) (string, error) {
	content := strings.TrimSpace(value)
	if content == "" || len([]rune(content)) > maxMessageLength {
		return "", errors.New("invalid message content")
	}
	return content, nil
}

func currentMemberID(c *gin.Context) string {
	memberID, _ := c.Get("member_id")
	return memberID.(string)
}

func (s *Service) memberByID(memberID string) (core.MemberData, error) {
	value, err := s.redis.Get(context.Background(), memberKey(memberID)).Result()
	if err != nil {
		return core.MemberData{}, err
	}
	var record redisMemberRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return core.MemberData{}, err
	}
	if record.ID == "" {
		return core.MemberData{}, errors.New("invalid redis member record")
	}
	return core.MemberData{Id: record.ID, Username: record.Username, Email: normalizeEmail(record.Email), PasswordHash: record.PasswordHash, Token: record.Token}, nil
}

func (s *Service) memberByEmail(email string) (core.MemberData, error) {
	ctx := context.Background()
	var cursor uint64
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "member:*", 0).Result()
		if err != nil {
			return core.MemberData{}, err
		}
		for _, key := range keys {
			if !isMemberRecordKey(key) {
				continue
			}
			memberID := strings.TrimPrefix(key, "member:")
			member, err := s.memberByID(memberID)
			if err != nil {
				return core.MemberData{}, err
			}
			if member.Email == normalizeEmail(email) {
				return member, nil
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return core.MemberData{}, redis.Nil
		}
	}
}

func isMemberRecordKey(key string) bool {
	return strings.HasPrefix(key, "member:") && strings.Count(key, ":") == 1 && strings.TrimPrefix(key, "member:") != ""
}

func isTokenRecordKey(key string) bool {
	return strings.HasPrefix(key, "token:") && strings.Count(key, ":") == 1 && strings.TrimPrefix(key, "token:") != ""
}

func (s *Service) validateToken(token string) (string, bool) {
	value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
	if err != nil {
		return "", false
	}
	var current tokenRecord
	if json.Unmarshal([]byte(value), &current) != nil || current.MemberID == "" {
		return "", false
	}
	return current.MemberID, true
}

func groupKey(groupID string) string         { return "group:" + groupID }
func groupMembersKey(groupID string) string  { return "group:" + groupID + ":members" }
func memberGroupsKey(memberID string) string { return "member:" + memberID + ":groups" }
func ownedGroupsKey(memberID string) string  { return "member:" + memberID + ":owned-groups" }
func messagesKey(groupID string) string      { return "group:" + groupID + ":messages" }
