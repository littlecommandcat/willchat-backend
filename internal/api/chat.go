package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"willchat-backend/internal/core"
)

const (
	maxGroupNameLength = 100
	maxMessageLength   = 4000
	maxGetAmount       = 50
	maxOwnedGroups     = 15
)

var errGroupLimitReached = errors.New("group limit reached")
var errAlreadyGroupMember = errors.New("already a group member")
var errMessageNotFound = errors.New("message not found")

type ChatService struct {
	db        *pgxpool.Pool
	snowflake *core.Snowflake
	auth      *Service
	hub       *core.Hub

	groupMu sync.Mutex
}

func NewChatService(db *pgxpool.Pool, snowflake *core.Snowflake, authService *Service, hub *core.Hub) *ChatService {
	return &ChatService{
		db:        db,
		snowflake: snowflake,
		auth:      authService,
		hub:       hub,
	}
}

func (s *ChatService) CreateGroup(c *gin.Context) {
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))
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
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if !canCreate {
		writeError(c, http.StatusConflict, "group_limit_reached", "a member can create at most 15 groups")
		return
	}

	groupID := s.snowflake.Next()
	group := groupRecord{
		GroupID:   groupID,
		Name:      name,
		OwnerID:   s.snowflake.ToSnowflake(memberID),
		CreatedAt: time.Now().UnixMilli(),
	}

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(context.Background(),
		`INSERT INTO groups (id, name, owner_id, created_at) VALUES ($1, $2, $3, $4)`,
		groupID, name, memberID, group.CreatedAt); err != nil {
		log.Printf("db group write failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if _, err := tx.Exec(context.Background(),
		`INSERT INTO group_members (group_id, member_id) VALUES ($1, $2)`,
		groupID, memberID); err != nil {
		log.Printf("db group owner membership write failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if err := tx.Commit(context.Background()); err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	log.Printf("[group] create succeeded group_id=%s member_id=%s name=%q", groupID, memberID, name)

	s.hub.BroadcastOnlyMember(s.snowflake.ToSnowflake(memberID), core.WSEvent{
		Event:   core.WSEventEnum.CreateGroup,
		Payload: gin.H{"group": toGroupResponse(s.snowflake, group)},
	})
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": toGroupResponse(s.snowflake, group)}})
}

func (s *ChatService) GetGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	group, err := s.auth.getGroup(groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	}
	if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	ownerID := s.snowflake.ToSnowflake(group.OwnerID)
	memberData, err := s.auth.memberByID(ownerID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	groupResponse := toGroupResponse(s.snowflake, group)
	memberResponse, _ := s.auth.toMemberResponse(memberData, false)
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": groupResponse, "member": memberResponse}})
}

func (s *ChatService) JoinGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))

	if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	member, err := s.auth.memberByID(memberID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	tag, err := s.db.Exec(context.Background(),
		`INSERT INTO group_members (group_id, member_id) VALUES ($1, $2)
		 ON CONFLICT (group_id, member_id) DO NOTHING`,
		groupID, memberID)
	if err != nil {
		log.Printf("db group member write failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(c, http.StatusConflict, "already_group_member", "member is already a member of this group")
		return
	}

	groupRecord, err := s.auth.getGroup(groupID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	groupResponse := toGroupResponse(s.snowflake, groupRecord)
	memberResponse, _ := s.auth.toMemberResponse(member, true)
	log.Printf("[group] join succeeded group_id=%s member_id=%s", groupID, memberID)
	s.hub.AddGroup(memberID, groupID)
	s.hub.BroadcastOnlyGroup(groupID, core.WSEvent{
		Event: core.WSEventEnum.JoinGroup,
		Payload: gin.H{
			"group":  groupResponse,
			"member": memberResponse,
		},
	})
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": groupResponse, "member": memberResponse}})
}

func (s *ChatService) LeaveGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))

	group, err := s.auth.getGroup(groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	}
	if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if s.snowflake.ToSnowflake(group.OwnerID) == memberID {
		writeError(c, http.StatusConflict, "owner_cannot_leave", "group owner cannot leave the group")
		return
	}

	tag, err := s.db.Exec(context.Background(),
		`DELETE FROM group_members WHERE group_id = $1 AND member_id = $2`,
		groupID, memberID)
	if err != nil {
		log.Printf("db group member removal failed group_id=%s member_id=%s: %v", groupID, memberID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(c, http.StatusNotFound, "not_group_member", "member is not a member of this group")
		return
	}

	memberData, err := s.auth.memberByID(memberID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	groupResponse := toGroupResponse(s.snowflake, group)
	memberResponse, _ := s.auth.toMemberResponse(memberData, true)
	log.Printf("[group] leave succeeded group_id=%s member_id=%s", groupID, memberID)
	s.hub.RemoveGroup(memberID, groupID)
	s.hub.BroadcastOnlyGroup(groupID, core.WSEvent{
		Event: core.WSEventEnum.LeaveGroup,
		Payload: gin.H{
			"group":  groupResponse,
			"member": memberResponse,
		},
	})
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": groupResponse, "member": memberResponse}})
}

func (s *ChatService) DeleteGroup(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))

	group, err := s.auth.getGroup(groupID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	}
	if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if s.snowflake.ToSnowflake(group.OwnerID) != memberID {
		writeError(c, http.StatusForbidden, "not_group_owner", "only the group owner can delete this group")
		return
	}

	groupResponse := toGroupResponse(s.snowflake, group)

	// Collect current members
	rows, err := s.db.Query(context.Background(),
		`SELECT member_id FROM group_members WHERE group_id = $1`, groupID)
	if err != nil {
		log.Printf("db group member read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	var members []string
	for rows.Next() {
		var mid string
		if err := rows.Scan(&mid); err != nil {
			rows.Close()
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
		members = append(members, mid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	if _, err := s.db.Exec(context.Background(),
		`DELETE FROM groups WHERE id = $1`, groupID); err != nil {
		log.Printf("db group deletion failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	memberData, err := s.auth.memberByID(memberID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	memberResponse, err := s.auth.toMemberResponse(memberData, true)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	log.Printf("[group] delete succeeded group_id=%s member_id=%s", groupID, memberID)
	s.hub.BroadcastOnlyGroup(groupID, core.WSEvent{
		Event: core.WSEventEnum.DeleteGroup,
		Payload: gin.H{
			"group":  groupResponse,
			"member": memberResponse,
		},
	})
	for _, groupMemberID := range members {
		s.hub.RemoveGroup(s.snowflake.ToSnowflake(groupMemberID), groupID)
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"group": groupResponse, "member": memberResponse}})
}

func (s *ChatService) SendMessage(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := currentMemberID(c)

	request, err := parseSendMessageRequest(c)
	if err != nil {
		writeSendMessageError(c, err)
		return
	}
	content, err := validateMessageContent(request.Content)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_message", "message content must be at most 4000 characters")
		return
	}
	if content == "" && len(request.Attachments) == 0 {
		writeError(c, http.StatusBadRequest, "invalid_message", "message must contain content or at least one attachment")
		return
	}

	if !s.isMember(groupID, memberID) {
		if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
			writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		} else {
			writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		}
		return
	}

	var replyTo *core.ReplyTo
	if request.ReplyToID != "" {
		original, err := s.findMessage(groupID, s.snowflake.ToSnowflake(request.ReplyToID))
		switch {
		case errors.Is(err, errMessageNotFound):
			writeError(c, http.StatusBadRequest, "reply_message_not_found", "the message being replied to was not found")
			return
		case err != nil:
			log.Printf("db message read failed group_id=%s message_id=%s: %v", groupID, request.ReplyToID, err)
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
		replyTo = &core.ReplyTo{Id: original.Id, AuthorId: original.AuthorId, Content: original.Content, Type: original.Type}
	}

	attachments, err := s.storeAttachments(request.Attachments)
	if err != nil {
		if errors.Is(err, errUnsupportedImage) {
			writeError(c, http.StatusBadRequest, "invalid_attachment", err.Error())
			return
		}
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	message, _, err := s.createMessage(groupID, memberID, content, attachments, replyTo)
	if err != nil {
		s.deleteAttachments(attachments)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	groupRecord, _ := s.auth.getGroup(groupID)
	responseGroup := toGroupResponse(s.snowflake, groupRecord)
	responseMessage := s.toMessageResponse(message)
	s.hub.BroadcastOnlyGroup(groupID, core.WSEvent{Event: core.WSEventEnum.SendMessage, Payload: gin.H{"message": responseMessage, "group": responseGroup}})
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{"message": responseMessage, "group": responseGroup}})
}

func (s *ChatService) DeleteMessage(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))
	messageID := c.Query("message_id")

	if messageID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "message_id is required")
		return
	}
	messageID = s.snowflake.ToSnowflake(messageID)

	if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	if !s.isMember(groupID, memberID) {
		writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		return
	}

	message, err := s.findMessage(groupID, messageID)
	if err != nil {
		writeError(c, http.StatusNotFound, "message_not_found", "message not found")
		return
	}

	if message.AuthorId != memberID {
		writeError(c, http.StatusForbidden, "not_message_author", "member is not message author")
		return
	}
	_, err = s.deleteMessage(groupID, messageID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "message_not_delete", err.Error())
		return
	}

	groupRecord, _ := s.auth.getGroup(groupID)
	responseGroup := toGroupResponse(s.snowflake, groupRecord)
	responseMessage := s.toMessageResponse(message)
	s.hub.BroadcastOnlyGroup(groupID, core.WSEvent{Event: core.WSEventEnum.DeleteMessage, Payload: gin.H{"message": responseMessage, "group": responseGroup}})
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"message": responseMessage, "group": responseGroup}})
}

func (s *ChatService) GetMessages(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(currentMemberID(c))

	if messageID := c.Query("message_id"); messageID != "" {
		s.getMessage(c, groupID, memberID, messageID)
		return
	}

	var request getMessagesRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "amount or message_id is required")
		return
	}

	amount := request.Amount
	if amount < 1 || amount > maxGetAmount {
		writeError(c, http.StatusBadRequest, "invalid_request", "message amount must be greater than 1 or empty")
		return
	}

	if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	if !s.isMember(groupID, memberID) {
		writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		return
	}

	messages, err := s.getMessages(groupID, amount)
	if err != nil {
		log.Printf("db messages read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	messageResponses := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		if message.Status == core.MessageStatusEnum.Deleted {
			continue
		}
		messageResponses = append(messageResponses, s.toMessageResponse(message))
	}

	c.JSON(http.StatusOK, core.Response{
		Status: "ok",
		Code:   "success",
		Data: gin.H{
			"messages": messageResponses,
			"amount":   amount,
		},
	})
}

func (s *ChatService) getMessage(c *gin.Context, groupID string, memberID string, messageID string) {
	groupID = s.snowflake.ToSnowflake(groupID)
	memberID = s.snowflake.ToSnowflake(memberID)
	messageID = s.snowflake.ToSnowflake(messageID)

	if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	if !s.isMember(groupID, memberID) {
		writeError(c, http.StatusForbidden, "not_group_member", "member is not a member of this group")
		return
	}

	message, err := s.findMessage(groupID, messageID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "message_not_found", "message not found")
		return
	}
	if err != nil {
		log.Printf("db message read failed group_id=%s message_id=%s: %v", groupID, messageID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if message.Status == core.MessageStatusEnum.Deleted {
		writeError(c, http.StatusNotFound, "message_not_found", "message not found")
		return
	}
	c.JSON(http.StatusOK, core.Response{
		Status: "ok",
		Code:   "success",
		Data: gin.H{
			"message": s.toMessageResponse(message),
		},
	})
}

func (s *ChatService) GetMember(c *gin.Context) {
	groupID := s.snowflake.ToSnowflake(c.Param("group_id"))
	memberID := s.snowflake.ToSnowflake(c.Param("member_id"))
	requesterID := currentMemberID(c)

	if groupID == "" || memberID == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "group_id and member_id are required")
		return
	}

	if _, err := s.auth.getGroup(groupID); errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "group_not_found", "group not found")
		return
	} else if err != nil {
		log.Printf("db group read failed group_id=%s: %v", groupID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
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
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}
	if err != nil {
		log.Printf("db member read failed member_id=%s: %v", memberID, err)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	groupRecord, err := s.auth.getGroup(groupID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	groupResponse := toGroupResponse(s.snowflake, groupRecord)

	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"group":  groupResponse,
		"member": memberProfile{ID: s.snowflake.ToMemberID(member.Id), Name: member.Name, CreatedAt: s.snowflake.ToTime(member.Id)},
	}})
}

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

func (s *ChatService) toMessageResponse(message core.Message) messageResponse {
	response := messageResponse{
		Content:   message.Content,
		Id:        s.snowflake.ToMessageID(message.Id),
		Timestamp: message.Timestamp,
		Type:      message.Type,
		Status:    message.Status,
	}
	if message.AuthorId != "" {
		response.AuthorId = s.snowflake.ToMemberID(message.AuthorId)
	}
	for _, attachment := range message.Attachments {
		external := attachmentResponse{
			Id:       attachment.Id,
			MimeType: attachment.MimeType,
			Width:    attachment.Width,
			Height:   attachment.Height,
			Size:     attachment.Size,
		}
		data, err := s.loadAttachment(attachment.Id)
		if err == nil {
			external.Data = base64.StdEncoding.EncodeToString(data)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("db attachment read failed attachment_id=%s: %v", attachment.Id, err)
		}
		response.Attachments = append(response.Attachments, external)
	}
	if message.ReplyTo != nil {
		response.ReplyTo = &replyToResponse{
			Id:       s.snowflake.ToMessageID(message.ReplyTo.Id),
			AuthorId: s.snowflake.ToMemberID(message.ReplyTo.AuthorId),
			Content:  message.ReplyTo.Content,
			Type:     message.ReplyTo.Type,
		}
	}
	return response
}

func (s *ChatService) createMessage(groupID, memberID, content string, attachments []core.Attachment, replyTo *core.ReplyTo) (core.Message, []byte, error) {
	id := s.snowflake.Next()
	message := core.Message{
		Id:          id,
		AuthorId:    s.snowflake.ToSnowflake(memberID),
		Timestamp:   s.snowflake.ToTime(id),
		Content:     content,
		Attachments: attachments,
		ReplyTo:     replyTo,
		Type:        core.MessageTypeEnum.Normal,
	}

	groupID = s.snowflake.ToSnowflake(groupID)
	authorID := s.snowflake.ToSnowflake(memberID)

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		return core.Message{}, nil, err
	}
	defer tx.Rollback(context.Background())

	var replyToID, replyToAuthorID, replyToContent *string
	var replyToType *int
	if replyTo != nil {
		rtID := s.snowflake.ToSnowflake(replyTo.Id)
		rtAuthor := s.snowflake.ToSnowflake(replyTo.AuthorId)
		rtContent := replyTo.Content
		rtType := int(replyTo.Type)
		replyToID = &rtID
		replyToAuthorID = &rtAuthor
		replyToContent = &rtContent
		replyToType = &rtType
	}

	if _, err := tx.Exec(context.Background(), `
		INSERT INTO messages (id, group_id, author_id, content, status, type, reply_to_id, reply_to_author_id, reply_to_content, reply_to_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		id, groupID, authorID, content, int(core.MessageStatusEnum.Normal), int(message.Type),
		replyToID, replyToAuthorID, replyToContent, replyToType, message.Timestamp); err != nil {
		log.Printf("db message write failed group_id=%s message_id=%s: %v", groupID, id, err)
		return core.Message{}, nil, err
	}

	for _, attachment := range attachments {
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO message_attachments (message_id, attachment_id) VALUES ($1, $2)`,
			id, attachment.Id); err != nil {
			log.Printf("db message attachment write failed message_id=%s attachment_id=%s: %v", id, attachment.Id, err)
			return core.Message{}, nil, err
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		return core.Message{}, nil, err
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return core.Message{}, nil, err
	}
	return message, payload, nil
}

func (s *ChatService) storeAttachments(images [][]byte) ([]core.Attachment, error) {
	attachments := make([]core.Attachment, 0, len(images))
	for _, raw := range images {
		compressed, mimeType, width, height, err := compressImage(raw, 1024)
		if err != nil {
			s.deleteAttachments(attachments)
			return nil, err
		}
		attachment, err := s.storeAttachment(compressed, mimeType, width, height)
		if err != nil {
			s.deleteAttachments(attachments)
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (s *ChatService) storeAttachment(data []byte, mimeType string, width, height int) (core.Attachment, error) {
	id := s.snowflake.Next()
	attachment := core.Attachment{Id: id, MimeType: mimeType, Width: width, Height: height, Size: len(data)}
	if _, err := s.db.Exec(context.Background(), `
		INSERT INTO attachments (id, mime_type, width, height, size, data)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id, mimeType, width, height, len(data), data); err != nil {
		return core.Attachment{}, err
	}
	return attachment, nil
}

func (s *ChatService) loadAttachment(attachmentID string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(context.Background(),
		`SELECT data FROM attachments WHERE id = $1`, attachmentID).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *ChatService) deleteAttachments(attachments []core.Attachment) {
	if len(attachments) == 0 {
		return
	}
	for _, attachment := range attachments {
		if _, err := s.db.Exec(context.Background(),
			`DELETE FROM attachments WHERE id = $1`, attachment.Id); err != nil {
			log.Printf("db attachment cleanup failed id=%s: %v", attachment.Id, err)
		}
	}
}

func (s *ChatService) loadMessageAttachments(messageID string) ([]core.Attachment, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT a.id, a.mime_type, a.width, a.height, a.size
		FROM attachments a
		JOIN message_attachments ma ON ma.attachment_id = a.id
		WHERE ma.message_id = $1
		ORDER BY a.id`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]core.Attachment, 0)
	for rows.Next() {
		var a core.Attachment
		if err := rows.Scan(&a.Id, &a.MimeType, &a.Width, &a.Height, &a.Size); err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}

func (s *ChatService) getMessages(groupID string, amount int) ([]core.Message, error) {
	groupID = s.snowflake.ToSnowflake(groupID)

	var query string
	var args []any
	if amount <= 0 {
		query = `
			SELECT id, author_id, content, status, type, reply_to_id, reply_to_author_id, reply_to_content, reply_to_type, created_at
			FROM messages WHERE group_id = $1 AND status <> $2
			ORDER BY created_at DESC, id DESC`
		args = []any{groupID, int(core.MessageStatusEnum.Deleted)}
	} else {
		query = `
			SELECT id, author_id, content, status, type, reply_to_id, reply_to_author_id, reply_to_content, reply_to_type, created_at
			FROM messages WHERE group_id = $1 AND status <> $2
			ORDER BY created_at DESC, id DESC
			LIMIT $3`
		args = []any{groupID, int(core.MessageStatusEnum.Deleted), amount}
	}

	rows, err := s.db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]core.Message, 0)
	for rows.Next() {
		var m core.Message
		var status, mtype int
		var replyToID, replyToAuthorID, replyToContent *string
		var replyToType *int
		if err := rows.Scan(&m.Id, &m.AuthorId, &m.Content, &status, &mtype,
			&replyToID, &replyToAuthorID, &replyToContent, &replyToType, &m.Timestamp); err != nil {
			return nil, err
		}
		m.Status = core.MessageStatus(status)
		m.Type = core.MessageType(mtype)
		if replyToID != nil {
			m.ReplyTo = &core.ReplyTo{
				Id:       *replyToID,
				AuthorId: *replyToAuthorID,
				Content:  *replyToContent,
				Type:     core.MessageType(*replyToType),
			}
		}
		attachments, err := s.loadMessageAttachments(m.Id)
		if err != nil {
			return nil, err
		}
		m.Attachments = attachments
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func validateMessageContent(value string) (string, error) {
	content := strings.TrimSpace(value)
	if len([]rune(content)) > maxMessageLength {
		return "", errors.New("invalid message content")
	}
	return content, nil
}

func (s *ChatService) findMessage(groupID, messageID string) (core.Message, error) {
	groupID = s.snowflake.ToSnowflake(groupID)
	messageID = s.snowflake.ToSnowflake(messageID)

	var m core.Message
	var status, mtype int
	var replyToID, replyToAuthorID, replyToContent *string
	var replyToType *int

	err := s.db.QueryRow(context.Background(), `
		SELECT id, author_id, content, status, type, reply_to_id, reply_to_author_id, reply_to_content, reply_to_type, created_at
		FROM messages WHERE id = $1 AND group_id = $2`,
		messageID, groupID).Scan(&m.Id, &m.AuthorId, &m.Content, &status, &mtype,
		&replyToID, &replyToAuthorID, &replyToContent, &replyToType, &m.Timestamp)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return core.Message{}, errMessageNotFound
		}
		return core.Message{}, err
	}
	m.Status = core.MessageStatus(status)
	m.Type = core.MessageType(mtype)
	if replyToID != nil {
		m.ReplyTo = &core.ReplyTo{
			Id:       *replyToID,
			AuthorId: *replyToAuthorID,
			Content:  *replyToContent,
			Type:     core.MessageType(*replyToType),
		}
	}
	attachments, err := s.loadMessageAttachments(m.Id)
	if err != nil {
		return core.Message{}, err
	}
	m.Attachments = attachments
	return m, nil
}

func (s *ChatService) deleteMessage(groupID, messageID string) (core.Message, error) {
	messageID = s.snowflake.ToSnowflake(messageID)
	message, err := s.findMessage(groupID, messageID)
	if err != nil {
		return message, err
	}
	message.Status = core.MessageStatusEnum.Deleted
	if _, err := s.db.Exec(context.Background(),
		`UPDATE messages SET status = $1 WHERE id = $2 AND group_id = $3`,
		int(core.MessageStatusEnum.Deleted), messageID, s.snowflake.ToSnowflake(groupID)); err != nil {
		message.Status = core.MessageStatusEnum.Normal
		return message, err
	}
	return message, nil
}

func (s *ChatService) isMember(groupID, memberID string) bool {
	var exists bool
	err := s.db.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND member_id = $2)`,
		s.snowflake.ToSnowflake(groupID), s.snowflake.ToSnowflake(memberID)).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

type sendMessageInput struct {
	Content     string
	ReplyToID   string
	Attachments [][]byte
}

func parseSendMessageRequest(c *gin.Context) (sendMessageInput, error) {
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		return parseMultipartMessage(c)
	}
	var request sendMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		return sendMessageInput{}, err
	}
	if len(request.Attachments) > maxAttachmentsPerMessage {
		return sendMessageInput{}, errTooManyAttachments
	}
	input := sendMessageInput{Content: request.Content, ReplyToID: strings.TrimSpace(request.ReplyTo)}
	for _, attachment := range request.Attachments {
		data, err := decodeBase64Image(attachment.Data)
		if err != nil {
			return sendMessageInput{}, err
		}
		if len(data) > maxAttachmentBytes {
			return sendMessageInput{}, errAttachmentTooLarge
		}
		input.Attachments = append(input.Attachments, data)
	}
	return input, nil
}

func parseMultipartMessage(c *gin.Context) (sendMessageInput, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMessageFormBytes)
	if err := c.Request.ParseMultipartForm(maxMultipartMemory); err != nil {
		return sendMessageInput{}, err
	}
	defer c.Request.MultipartForm.RemoveAll()

	files := c.Request.MultipartForm.File["attachments"]
	if len(files) > maxAttachmentsPerMessage {
		return sendMessageInput{}, errTooManyAttachments
	}
	input := sendMessageInput{
		Content:   c.Request.FormValue("content"),
		ReplyToID: strings.TrimSpace(c.Request.FormValue("reply_to")),
	}
	for _, header := range files {
		data, err := readUpload(header, maxAttachmentBytes)
		if err != nil {
			return sendMessageInput{}, err
		}
		input.Attachments = append(input.Attachments, data)
	}
	return input, nil
}

func readUpload(header *multipart.FileHeader, limit int64) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errAttachmentTooLarge
	}
	return data, nil
}

func decodeBase64Image(value string) ([]byte, error) {
	payload := strings.TrimSpace(value)
	if index := strings.Index(payload, ","); strings.HasPrefix(payload, "data:") && index != -1 {
		payload = payload[index+1:]
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, errUnsupportedImage
	}
	return data, nil
}

func writeSendMessageError(c *gin.Context, err error) {
	var maxBytesError *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesError):
		writeError(c, http.StatusRequestEntityTooLarge, "attachment_too_large", "attachments must not exceed 25 MB in total")
	case errors.Is(err, errTooManyAttachments):
		writeError(c, http.StatusBadRequest, "too_many_attachments", "a message can have at most 4 attachments")
	case errors.Is(err, errAttachmentTooLarge):
		writeError(c, http.StatusBadRequest, "attachment_too_large", "each image must be 5 MB or smaller")
	case errors.Is(err, errUnsupportedImage):
		writeError(c, http.StatusBadRequest, "invalid_attachment", err.Error())
	default:
		writeError(c, http.StatusBadRequest, "invalid_request", "request body is invalid")
	}
}

func currentMemberID(c *gin.Context) string {
	memberID, _ := c.Get("member_id")
	return memberID.(string)
}
