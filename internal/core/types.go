package core

type MessageType int
type MessageStatus int
type wsEventType string

// Attachment
type Attachment struct {
	Id       string `json:"id"`
	MimeType string `json:"mime_type"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Size     int    `json:"size"`
}

// ReplyTo snapshot
type ReplyTo struct {
	Id       string      `json:"id"`
	AuthorId string      `json:"authorid"`
	Content  string      `json:"content"`
	Type     MessageType `json:"type"`
}

const (
	eventMessageType string = "message_"
	eventGroupType   string = "group_"
	eventMemberType  string = "member_"
)

type messageTypeEnum struct {
	Normal    MessageType
	Reference MessageType
	Dev       MessageType
}

var MessageTypeEnum = messageTypeEnum{
	Normal:    0,
	Reference: 1,
	Dev:       2,
}

type messageStatusEnum struct {
	Normal  MessageStatus
	Deleted MessageStatus
	Blocked MessageStatus
}

var MessageStatusEnum = messageStatusEnum{
	Normal:  0,
	Deleted: 1,
	Blocked: 2,
}

type eventType struct {
	SendMessage   wsEventType
	DeleteMessage wsEventType
	ProfileUpdate wsEventType
	JoinGroup     wsEventType
	LeaveGroup    wsEventType
	CreateGroup   wsEventType
	DeleteGroup   wsEventType
}

var WSEventEnum = eventType{
	// Message
	SendMessage:   wsEventType(eventMessageType + "send"),
	DeleteMessage: wsEventType(eventMessageType + "delete"),

	// Member
	ProfileUpdate: wsEventType(eventMemberType + "update"),

	// Group
	JoinGroup:   wsEventType(eventGroupType + "join"),
	LeaveGroup:  wsEventType(eventGroupType + "leave"),
	CreateGroup: wsEventType(eventGroupType + "create"),
	DeleteGroup: wsEventType(eventGroupType + "delete"),
}

type MemberData struct {
	Id           string            `json:"id"`
	Username     string            `json:"username"`
	Email        string            `json:"email"`
	PasswordHash string            `json:"-"`
	Token        string            `json:"-"`
	Block        []string          `json:"block"`
	Read         map[string]string `json:"read"`
}

type TokenData struct {
	Id string `json:"id"`
}

type Message struct {
	Content     string        `json:"content"`
	Attachments []Attachment  `json:"attachments,omitempty"`
	ReplyTo     *ReplyTo      `json:"reply_to,omitempty"`
	Id          string        `json:"id"`
	AuthorId    string        `json:"authorid"`
	Timestamp   int64         `json:"timestamp"`
	Type        MessageType   `json:"type"`
	Status      MessageStatus `json:"status"`
}

type Response struct {
	Status  string         `json:"status"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data" binding:"omitempty"`
}

type WSEvent struct {
	// Event   string `json:"event"`
	Event   wsEventType `json:"event"`
	Payload any         `json:"payload,omitempty"`
}

type WSMessage struct {
	Content string `json:"content"`
}
