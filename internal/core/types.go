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
