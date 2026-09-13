package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	errInvalidCredentials = errors.New("invalid email or password")
	errRedisUnavailable   = errors.New("redis is unavailable")
)

const maxUsernameLength = 50

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

type userResponse struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	GroupIDs []string `json:"group_ids"`
}

type tokenRecord struct {
	UserID string `json:"user_id"`
}

type redisUserRecord struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	Email        string   `json:"email"`
	PasswordHash string   `json:"password_hash"`
	GroupIDs     []string `json:"group_ids"`
	Token        string   `json:"token"`
}

type Service struct {
	redis     *redis.Client
	snowflake *core.Snowflake
	hub       *core.Hub
	mu        sync.RWMutex
	users     map[string]core.UserData
}

func NewService(redisClient *redis.Client, snowflake *core.Snowflake) (*Service, error) {
	service := &Service{
		redis:     redisClient,
		snowflake: snowflake,
		users:     make(map[string]core.UserData),
	}
	if err := service.loadUsers(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) SetHub(hub *core.Hub) {
	s.hub = hub
}

func (s *Service) Register(c *gin.Context) {
	var request registerRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "username is required, email must be valid and password must be at least 8 characters")
		return
	}
	request.Username = strings.TrimSpace(request.Username)
	request.Email = normalizeEmail(request.Email)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[request.Email]; exists {
		writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to create user")
		return
	}
	user := core.UserData{Id: s.snowflake.Next(), Username: request.Username, Email: request.Email, PasswordHash: string(hash), GroupIDs: []string{}}

	token, err := s.createToken(user.Id)
	if err != nil {
		delete(s.users, user.Email)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	user.Token = token
	s.users[user.Email] = user
	if err := s.storeUser(user); err != nil {
		_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
		delete(s.users, user.Email)
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": token,
		"user":  toUserResponse(user),
	}})
}

func (s *Service) Login(c *gin.Context) {
	var request credentialsRequest
	if !bindCredentials(c, &request) {
		return
	}
	request.Email = normalizeEmail(request.Email)

	s.mu.RLock()
	user, exists := s.users[request.Email]
	s.mu.RUnlock()
	if !exists || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", errInvalidCredentials.Error())
		return
	}
	token := user.Token
	if token == "" || !s.tokenBelongsToUser(token, user.Id) {
		var err error
		token, err = s.createToken(user.Id)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		user.Token = token
		s.mu.Lock()
		s.users[request.Email] = user
		storeErr := s.storeUser(user)
		s.mu.Unlock()
		if storeErr != nil {
			_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
	}

	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": token,
		"user":  toUserResponse(user),
	}})
}

func (s *Service) Logout(c *gin.Context) {
	token, ok := c.Get("token")
	tokenValue, validType := token.(string)
	if !ok || !validType || tokenValue == "" {
		writeError(c, http.StatusUnauthorized, "missing_token", "a bearer token is required")
		return
	}
	if err := s.redis.Del(context.Background(), tokenKey(tokenValue)).Err(); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if userID, ok := c.Get("user_id"); ok && s.hub != nil {
		s.hub.CloseUser(userID.(string))
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success"})
}

func (s *Service) RequireToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c)
		if !ok {
			writeError(c, http.StatusUnauthorized, "missing_token", "a bearer token is required")
			c.Abort()
			return
		}
		value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
		if err == redis.Nil {
			writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
			c.Abort()
			return
		}
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			c.Abort()
			return
		}
		var currentToken tokenRecord
		if json.Unmarshal([]byte(value), &currentToken) != nil || currentToken.UserID == "" {
			writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
			c.Abort()
			return
		}
		c.Set("token", token)
		c.Set("user_id", currentToken.UserID)
		c.Next()
	}
}

func (s *Service) Me(c *gin.Context) {
	userID, _ := c.Get("user_id")
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Id == userID {
			c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: toUserResponse(user)})
			return
		}
	}
	writeError(c, http.StatusNotFound, "user_not_found", "user not found")
}

func (s *Service) UpdateProfile(c *gin.Context) {
	var request updateProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "email must be valid and username must be provided")
		return
	}
	if request.Username == nil && request.Email == nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "username or email is required")
		return
	}

	userID, _ := c.Get("user_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	var currentEmail string
	var user core.UserData
	for email, candidate := range s.users {
		if candidate.Id == userID {
			currentEmail = email
			user = candidate
			break
		}
	}
	if currentEmail == "" {
		writeError(c, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	if request.Username != nil {
		username := strings.TrimSpace(*request.Username)
		if username == "" || len([]rune(username)) > maxUsernameLength {
			writeError(c, http.StatusBadRequest, "invalid_username", "username must be between 1 and 50 characters")
			return
		}
		user.Username = username
	}
	newEmail := user.Email
	if request.Email != nil {
		newEmail = normalizeEmail(*request.Email)
		if newEmail == "" {
			writeError(c, http.StatusBadRequest, "invalid_email", "email must be valid")
			return
		}
		if existing, exists := s.users[newEmail]; exists && existing.Id != user.Id {
			writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
			return
		}
		user.Email = newEmail
	}

	if err := s.storeUser(user); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if newEmail != currentEmail {
		delete(s.users, currentEmail)
	}
	s.users[newEmail] = user
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: toUserResponse(user)})
}

func (s *Service) tokenBelongsToUser(token, userID string) bool {
	value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
	if err != nil {
		return false
	}
	var record tokenRecord
	return json.Unmarshal([]byte(value), &record) == nil && record.UserID == userID
}

func toUserResponse(user core.UserData) userResponse {
	return userResponse{ID: user.Id, Username: user.Username, Email: user.Email, GroupIDs: user.GroupIDs}
}

func (s *Service) createToken(userID string) (string, error) {
	token := newToken()
	key := tokenKey(token)
	payload, err := json.Marshal(tokenRecord{UserID: userID})
	if err != nil {
		log.Printf("redis token marshal failed key=%s: %v", key, err)
		return "", err
	}
	// expiration=0 token
	if err := s.redis.Set(context.Background(), key, payload, 0).Err(); err != nil {
		log.Printf("redis token write failed key=%s: %v", key, err)
		return "", err
	}
	storedPayload, err := s.redis.Get(context.Background(), key).Result()
	if err != nil {
		log.Printf("redis token read-back failed key=%s: %v", key, err)
		return "", err
	}
	if storedPayload != string(payload) {
		log.Printf("redis token read-back mismatch key=%s", key)
		return "", errors.New("redis token verification failed")
	}
	log.Printf("redis token stored user_id=%s", userID)
	return token, nil
}

func (s *Service) storeUser(user core.UserData) error {
	key := userKey(user.Id)
	payload, err := json.Marshal(redisUserRecord{ID: user.Id, Username: user.Username, Email: user.Email, PasswordHash: user.PasswordHash, GroupIDs: user.GroupIDs, Token: user.Token})
	if err != nil {
		log.Printf("redis user marshal failed key=%s: %v", key, err)
		return err
	}
	if err := s.redis.Set(context.Background(), key, payload, 0).Err(); err != nil {
		log.Printf("redis user write failed key=%s: %v", key, err)
		return err
	}
	storedPayload, err := s.redis.Get(context.Background(), key).Result()
	if err != nil {
		log.Printf("redis user read-back failed key=%s: %v", key, err)
		return err
	}
	if storedPayload != string(payload) {
		log.Printf("redis user read-back mismatch key=%s", key)
		return errors.New("redis user verification failed")
	}
	log.Printf("redis user stored key=%s", key)
	return nil
}

func (s *Service) canCreateGroup(userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Id == userID {
			return len(user.GroupIDs) < maxOwnedGroups
		}
	}
	return false
}

func (s *Service) addOwnedGroup(userID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for email, user := range s.users {
		if user.Id != userID {
			continue
		}
		if len(user.GroupIDs) >= maxOwnedGroups {
			return errGroupLimitReached
		}
		user.GroupIDs = append(user.GroupIDs, groupID)
		if err := s.storeUser(user); err != nil {
			return err
		}
		s.users[email] = user
		return nil
	}
	return errors.New("user not found")
}

func (s *Service) loadUsers() error {
	ctx := context.Background()
	var cursor uint64
	loaded := 0
	for {
		keys, nextCursor, err := s.redis.Scan(ctx, cursor, "user:*", 0).Result()
		if err != nil {
			log.Printf("redis user scan failed: %v", err)
			return err
		}
		for _, key := range keys {
			if !isUserRecordKey(key) {
				continue
			}
			value, err := s.redis.Get(ctx, key).Result()
			if err != nil {
				log.Printf("redis user load failed key=%s: %v", key, err)
				return err
			}
			var record redisUserRecord
			if err := json.Unmarshal([]byte(value), &record); err != nil {
				log.Printf("redis user decode failed key=%s: %v", key, err)
				return err
			}
			if record.ID == "" || record.Email == "" {
				log.Printf("redis user record invalid key=%s", key)
				return errors.New("invalid redis user record")
			}
			normalizedEmail := normalizeEmail(record.Email)
			if existing, exists := s.users[normalizedEmail]; exists {
				log.Printf("redis duplicate user email=%s existing_id=%s duplicate_id=%s", normalizedEmail, existing.Id, record.ID)
				return errors.New("duplicate redis user email")
			}
			if record.GroupIDs == nil {
				record.GroupIDs = []string{}
			}
			s.users[normalizedEmail] = core.UserData{
				Id:           record.ID,
				Username:     record.Username,
				Email:        normalizedEmail,
				PasswordHash: record.PasswordHash,
				GroupIDs:     record.GroupIDs,
				Token:        record.Token,
			}
			loaded++
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	log.Printf("redis users loaded count=%d", loaded)
	return nil
}

func isUserRecordKey(key string) bool {
	return strings.HasPrefix(key, "user:") && strings.Count(key, ":") == 1 && strings.TrimPrefix(key, "user:") != ""
}

func bindCredentials(c *gin.Context, request *credentialsRequest) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "email must be valid and password must be at least 8 characters")
		return false
	}
	return true
}

func bearerToken(c *gin.Context) (string, bool) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	return func() (string, bool) {
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != "" {
			return parts[1], true
		}
		return "", false
	}()
}

func tokenKey(token string) string       { return "token:" + token }
func userKey(userID string) string       { return "user:" + userID }
func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func newToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, core.Response{Status: "error", Code: code, Data: gin.H{"message": message}})
}
