package api

import "willchat-backend/internal/core"

// Auth requests

type credentialsRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=1,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Code     string `json:"code" binding:"required,len=6,numeric"`
}

type verificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type updateProfileRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email" binding:"omitempty,email"`
	Code     string  `json:"code"`
}

type resetPasswordRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

// Auth responses and records

type memberResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	// GroupIDs     []string      `json:"group_ids"`
	GroupRecords []groupResponse `json:"group_data"`
}

// memberProfile
type memberProfile struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"created_at"`
}

type tokenRecord struct {
	MemberID string `json:"member_id"`
}

type redisMemberRecord struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	Token        string `json:"token"`
}

// Chat requests and records
// Maybe it failed to get data (idk)
type groupRecord struct {
	GroupID   string `json:"group_id"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

type createGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

type sendMessageRequest struct {
	Content string `json:"content"`
	// ReplyTo
	ReplyTo string `json:"reply_to"`
	// Attachments base64-encoded
	Attachments []attachmentRequest `json:"attachments"`
}

// attachmentRequest
type attachmentRequest struct {
	Data string `json:"data"`
}

type getMessagesRequest struct {
	Amount int `form:"amount" binding:"required"`
}

type getMessageRequest struct {
	MessageID string `form:"message_id" binding:"required"`
}

type groupResponse struct {
	GroupID   string `json:"group_id"`
	Name      string `json:"name,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

type messageResponse struct {
	Content     string               `json:"content"`
	Attachments []attachmentResponse `json:"attachments,omitempty"`
	ReplyTo     *replyToResponse     `json:"reply_to,omitempty"`
	Id          string               `json:"id"`
	AuthorId    string               `json:"authorid"`
	Timestamp   int64                `json:"timestamp"`
	Type        core.MessageType     `json:"type"`
}

// attachmentResponse
type attachmentResponse struct {
	Id       string `json:"id"`
	MimeType string `json:"mime_type"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Size     int    `json:"size"`
	Data     string `json:"data"`
}

// replyToResponse
type replyToResponse struct {
	Id       string           `json:"id"`
	AuthorId string           `json:"authorid"`
	Content  string           `json:"content"`
	Type     core.MessageType `json:"type"`
}
