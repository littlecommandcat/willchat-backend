package core

type UserData struct {
	Id           string   `json:"id"`
	Username     string   `json:"username"`
	Email        string   `json:"email"`
	PasswordHash string   `json:"-"`
	GroupIDs     []string `json:"group_ids"`
	Token        string   `json:"-"`
}

type TokenData struct {
	Id string `json:"id"`
}

type Message struct {
	Content string   `json:"content"`
	Id      string   `json:"id"`
	Author  UserData `json:"author"`
}
