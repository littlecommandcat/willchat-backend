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

type resetPasswordRequest struct {
	Password    string `json:"password"`
	NewPassword string `json:"new_password"`
}

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

type Service struct {
	redis     *redis.Client
	snowflake *core.Snowflake
	hub       *core.Hub
	mu        sync.RWMutex
}

func NewService(redisClient *redis.Client, snowflake *core.Snowflake) (*Service, error) {
	service := &Service{
		redis:     redisClient,
		snowflake: snowflake,
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
	if _, err := s.userByEmail(request.Email); err == nil {
		writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
		return
	} else if !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to create user")
		return
	}
	user := core.UserData{Id: s.snowflake.Next(), Username: request.Username, Email: request.Email, PasswordHash: string(hash)}

	token, err := s.createToken(user.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	user.Token = token
	if err := s.storeUser(user); err != nil {
		_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	responseUser, err := s.toUserResponse(user)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": token,
		"user":  responseUser,
	}})
}

func (s *Service) Login(c *gin.Context) {
	var request credentialsRequest
	if !bindCredentials(c, &request) {
		return
	}
	request.Email = normalizeEmail(request.Email)

	user, err := s.userByEmail(request.Email)
	if err != nil && !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if errors.Is(err, redis.Nil) || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", errInvalidCredentials.Error())
		return
	}
	token := user.Token
	if token == "" || !s.tokenBelongsToUser(token, user.Id) {
		token, err = s.createToken(user.Id)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		user.Token = token
		storeErr := s.storeUser(user)
		if storeErr != nil {
			_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
	}

	responseUser, err := s.toUserResponse(user)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": token,
		"user":  responseUser,
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
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{}})
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
	user, err := s.userByID(userID.(string))
	if err == nil {
		responseUser, responseErr := s.toUserResponse(user)
		if responseErr != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"user": responseUser}})
		return
	}
	if !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
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
	user, err := s.userByID(userID.(string))
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
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
		if existing, err := s.userByEmail(newEmail); err == nil {
			if existing.Id != user.Id {
				writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
				return
			}
		} else if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		user.Email = newEmail
	}

	if err := s.storeUser(user); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	responseUser, responseErr := s.toUserResponse(user)
	if responseErr != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"user": responseUser}})
}

func (s *Service) ResetPassword(c *gin.Context) {
	var request resetPasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
		return
	}
	newPassword := request.Password
	if newPassword == "" {
		newPassword = request.NewPassword
	}
	if len([]rune(newPassword)) < 8 {
		writeError(c, http.StatusBadRequest, "invalid_password", "password must be at least 8 characters")
		return
	}

	userID, ok := c.Get("user_id")
	if !ok {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}
	userIDValue, ok := userID.(string)
	if !ok || strings.TrimSpace(userIDValue) == "" {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}
	oldToken, ok := c.Get("token")
	if !ok {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}
	oldTokenValue, ok := oldToken.(string)
	if !ok || strings.TrimSpace(oldTokenValue) == "" {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	user, err := s.userByID(userIDValue)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		writeError(c, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	if !s.tokenBelongsToUser(oldTokenValue, user.Id) {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to reset password")
		return
	}

	newToken, err := s.createToken(user.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	user.PasswordHash = string(hash)
	user.Token = newToken
	if err := s.storeUser(user); err != nil {
		_ = s.redis.Del(context.Background(), tokenKey(newToken)).Err()
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.redis.Del(context.Background(), tokenKey(oldTokenValue)).Err(); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if s.hub != nil {
		s.hub.CloseUser(user.Id)
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": newToken,
	}})
}

func (s *Service) tokenBelongsToUser(token, userID string) bool {
	value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
	if err != nil {
		return false
	}
	var record tokenRecord
	return json.Unmarshal([]byte(value), &record) == nil && record.UserID == userID
}

func (s *Service) toUserResponse(user core.UserData) (userResponse, error) {
	groupIDs, err := s.redis.SMembers(context.Background(), ownedGroupsKey(user.Id)).Result()
	if err != nil {
		return userResponse{}, err
	}
	groupData := make([]groupRecord, len(groupIDs))
	for i := 0; i < len(groupIDs); i += 1 {
		if groupIDs[i] == "" {
			continue
		}
		group, err := s.getGroup(groupIDs[i])
		if err != nil {
			log.Fatal(err)
			groupRe := groupRecord{
				ID: groupIDs[i],
			}
			groupData = append(groupData, groupRe)
			continue
		}
		log.Printf("ID: %s", groupIDs[i])
		groupData = append(groupData, group)
	}
	return userResponse{ID: user.Id, Username: user.Username, Email: user.Email, GroupRecords: groupData[1:]}, nil
}

func (s *Service) getGroup(groupID string) (groupRecord, error) {
	value, err := s.redis.Get(context.Background(), groupKey(groupID)).Result()
	if err != nil {
		return groupRecord{}, err
	}
	var group groupRecord
	if err := json.Unmarshal([]byte(value), &group); err != nil {
		return groupRecord{}, err
	}
	if group.GroupID == "" {
		group.GroupID = group.ID
	}
	return group, nil
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
	payload, err := json.Marshal(redisUserRecord{ID: user.Id, Username: user.Username, Email: user.Email, PasswordHash: user.PasswordHash, Token: user.Token})
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

func (s *Service) canCreateGroup(userID string) (bool, error) {
	count, err := s.redis.SCard(context.Background(), ownedGroupsKey(userID)).Result()
	if err != nil {
		return false, err
	}
	return count < maxOwnedGroups, nil
}

func (s *Service) addOwnedGroup(userID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	count, err := s.redis.SCard(context.Background(), ownedGroupsKey(userID)).Result()
	if err != nil {
		return err
	}
	if count >= maxOwnedGroups {
		return errGroupLimitReached
	}
	return s.redis.SAdd(context.Background(), ownedGroupsKey(userID), groupID).Err()
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
