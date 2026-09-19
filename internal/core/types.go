package core

type MessageType int
type wsEventType string

type messageTypeEnum struct {
	Normal MessageType
	Dev    MessageType
}

var MessageTypeEnum = messageTypeEnum{
	Normal: 0,
	Dev:    1,
}

type eventType struct {
	SendMessage   wsEventType
	DeleteMessage wsEventType
	ProfileUpdate wsEventType
	JoinGroup     wsEventType
	LeaveGroup    wsEventType
	DeleteGroup   wsEventType
}

var WSEventEnum = eventType{
	SendMessage:   "send_message",
	DeleteMessage: "delete_message",
	ProfileUpdate: "update_profile",
	JoinGroup:     "join_group",
	LeaveGroup:    "leave_group",
	DeleteGroup:   "delete_group",
}

type UserData struct {
	Id           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Token        string `json:"-"`
}

type TokenData struct {
	Id string `json:"id"`
}

type Message struct {
	Content   string      `json:"content"`
	Id        string      `json:"id"`
	AuthorId  int         `json:"authorid"`
	Timestamp int64       `json:"timestamp"`
	Type      MessageType `json:"type"`
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
	GroupID string      `json:"groupid"`
	Data    any         `json:"data,omitempty"`
	Content string      `json:"content,omitempty"`
}

type WSMessage struct {
	Content string `json:"content"`
}
