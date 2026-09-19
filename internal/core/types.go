package core

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
	Content  string `json:"content"`
	Id       string `json:"id"`
	AuthorId int    `json:"authorid"`
}

type Response struct {
	Status  string         `json:"status"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data" binding:"omitempty"`
}

type WSEvent struct {
	Event   string `json:"event"`
	GroupID string `json:"groupid"`
	Data    any    `json:"data,omitempty"`
	Content string `json:"content,omitempty"`
}

type WSMessage struct {
	Content string `json:"content"`
}
