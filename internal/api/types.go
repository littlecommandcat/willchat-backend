package api

// Auth requests

type credentialsRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=1,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type updateProfileRequest struct {
	Username *string `json:"username"`
	Email    *string `json:"email" binding:"omitempty,email"`
}

type resetPasswordRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

// Auth responses and records

type userResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	// GroupIDs     []string      `json:"group_ids"`
	GroupRecords []groupRecord `json:"group_data"`
}

type tokenRecord struct {
	UserID string `json:"user_id"`
}

type redisUserRecord struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	Token        string `json:"token"`
}

// Chat requests and records

// Maybe it failed to get data
type groupRecord struct {
	GroupID   string `json:"group_id"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	OwnerID   string `json:"owner_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type createGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

type sendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type getMessageRequest struct {
	Amount int `form:"amount" binding:"required"`
}
