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
	if _, err := s.memberByEmail(request.Email); err == nil {
		writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
		return
	} else if !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to create member")
		return
	}
	member := core.MemberData{Id: s.snowflake.Next(), Username: request.Username, Email: request.Email, PasswordHash: string(hash)}

	token, err := s.createToken(member.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	member.Token = token
	if err := s.storeMember(member); err != nil {
		_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}

	responseMember, err := s.toMemberResponse(member)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusCreated, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token":  token,
		"member": responseMember,
	}})
}

func (s *Service) Login(c *gin.Context) {
	var request credentialsRequest
	if !bindCredentials(c, &request) {
		return
	}
	request.Email = normalizeEmail(request.Email)

	member, err := s.memberByEmail(request.Email)
	if err != nil && !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if errors.Is(err, redis.Nil) || bcrypt.CompareHashAndPassword([]byte(member.PasswordHash), []byte(request.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", errInvalidCredentials.Error())
		return
	}
	token := member.Token
	if token == "" || !s.tokenBelongsToMember(token, member.Id) {
		token, err = s.createToken(member.Id)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		member.Token = token
		storeErr := s.storeMember(member)
		if storeErr != nil {
			_ = s.redis.Del(context.Background(), tokenKey(token)).Err()
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
	}

	responseMember, err := s.toMemberResponse(member)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token":  token,
		"member": responseMember,
	}})
}

// I dont think  we need logout route if we have websocket

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
		if json.Unmarshal([]byte(value), &currentToken) != nil || currentToken.MemberID == "" {
			writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
			c.Abort()
			return
		}
		c.Set("token", token)
		c.Set("member_id", currentToken.MemberID)
		c.Next()
	}
}

func (s *Service) Me(c *gin.Context) {
	memberID, _ := c.Get("member_id")
	member, err := s.memberByID(memberID.(string))
	if err == nil {
		responseMember, responseErr := s.toMemberResponse(member)
		if responseErr != nil {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"member": responseMember}})
		return
	}
	if !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	writeError(c, http.StatusNotFound, "member_not_found", "member not found")
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

	memberID, _ := c.Get("member_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	member, err := s.memberByID(memberID.(string))
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}

	if request.Username != nil {
		username := strings.TrimSpace(*request.Username)
		if username == "" || len([]rune(username)) > maxUsernameLength {
			writeError(c, http.StatusBadRequest, "invalid_username", "username must be between 1 and 50 characters")
			return
		}
		member.Username = username
	}
	newEmail := member.Email
	if request.Email != nil {
		newEmail = normalizeEmail(*request.Email)
		if newEmail == "" {
			writeError(c, http.StatusBadRequest, "invalid_email", "email must be valid")
			return
		}
		if existing, err := s.memberByEmail(newEmail); err == nil {
			if existing.Id != member.Id {
				writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
				return
			}
		} else if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		member.Email = newEmail
	}

	if err := s.storeMember(member); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	responseMember, responseErr := s.toMemberResponse(member)
	if responseErr != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"member": responseMember}})
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

	memberID, ok := c.Get("member_id")
	if !ok {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}
	memberIDValue, ok := memberID.(string)
	if !ok || strings.TrimSpace(memberIDValue) == "" {
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
	member, err := s.memberByID(memberIDValue)
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
			return
		}
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}

	if !s.tokenBelongsToMember(oldTokenValue, member.Id) {
		writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to reset password")
		return
	}

	newToken, err := s.createToken(member.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	member.PasswordHash = string(hash)
	member.Token = newToken
	if err := s.storeMember(member); err != nil {
		_ = s.redis.Del(context.Background(), tokenKey(newToken)).Err()
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if err := s.redis.Del(context.Background(), tokenKey(oldTokenValue)).Err(); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if s.hub != nil {
		s.hub.CloseMember(member.Id)
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token": newToken,
	}})
}

func (s *Service) tokenBelongsToMember(token, memberID string) bool {
	value, err := s.redis.Get(context.Background(), tokenKey(token)).Result()
	if err != nil {
		return false
	}
	var record tokenRecord
	return json.Unmarshal([]byte(value), &record) == nil && record.MemberID == memberID
}

func (s *Service) toMemberResponse(member core.MemberData) (memberResponse, error) {
	groupIDs, err := s.redis.SMembers(context.Background(), ownedGroupsKey(member.Id)).Result()
	if err != nil {
		return memberResponse{}, err
	}
	groupData := make([]groupResponse, 0, len(groupIDs))
	for i := 0; i < len(groupIDs); i += 1 {
		if groupIDs[i] == "" {
			continue
		}
		group, err := s.getGroup(groupIDs[i])
		if err != nil {
			log.Printf("getGroup %s failed: %v", groupIDs[i], err)
			continue
		}
		log.Printf("ID: %s", groupIDs[i])
		groupData = append(groupData, toGroupResponse(s.snowflake, group))
	}
	return memberResponse{ID: s.snowflake.ToMemberID(member.Id), Username: member.Username, Email: member.Email, GroupRecords: groupData}, nil
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

func (s *Service) createToken(memberID string) (string, error) {
	token := newToken()
	key := tokenKey(token)
	payload, err := json.Marshal(tokenRecord{MemberID: memberID})
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
	log.Printf("redis token stored member_id=%s", memberID)
	return token, nil
}

func (s *Service) storeMember(member core.MemberData) error {
	key := memberKey(member.Id)
	payload, err := json.Marshal(redisMemberRecord{ID: member.Id, Username: member.Username, Email: member.Email, PasswordHash: member.PasswordHash, Token: member.Token})
	if err != nil {
		log.Printf("redis member marshal failed key=%s: %v", key, err)
		return err
	}
	if err := s.redis.Set(context.Background(), key, payload, 0).Err(); err != nil {
		log.Printf("redis member write failed key=%s: %v", key, err)
		return err
	}
	storedPayload, err := s.redis.Get(context.Background(), key).Result()
	if err != nil {
		log.Printf("redis member read-back failed key=%s: %v", key, err)
		return err
	}
	if storedPayload != string(payload) {
		log.Printf("redis member read-back mismatch key=%s", key)
		return errors.New("redis member verification failed")
	}
	log.Printf("redis member stored key=%s", key)
	return nil
}

func (s *Service) canCreateGroup(memberID string) (bool, error) {
	count, err := s.redis.SCard(context.Background(), ownedGroupsKey(memberID)).Result()
	if err != nil {
		return false, err
	}
	return count < maxOwnedGroups, nil
}

func (s *Service) addOwnedGroup(memberID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	count, err := s.redis.SCard(context.Background(), ownedGroupsKey(memberID)).Result()
	if err != nil {
		return err
	}
	if count >= maxOwnedGroups {
		return errGroupLimitReached
	}
	return s.redis.SAdd(context.Background(), ownedGroupsKey(memberID), groupID).Err()
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
func memberKey(memberID string) string   { return "member:" + memberID }
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
