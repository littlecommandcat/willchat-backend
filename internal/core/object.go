package core

type UserData struct {
	Id           string   `json:"id"`
	Email        string   `json:"email"`
	PasswordHash string   `json:"-"`
	GroupIDs     []string `json:"group_ids"`
}

type TokenData struct {
	Id string `json:"id"`
}
