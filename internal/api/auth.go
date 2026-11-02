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

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"willchat-backend/internal/core"
)

var (
	errInvalidCredentials  = errors.New("invalid email or password")
	errDatabaseUnavailable = errors.New("database is unavailable")
)

const maxnameLength = 50

// queryExecer is satisfied by both *pgxpool.Pool and pgx.Tx so the same
// data-access helpers can run inside a transaction or against the pool.
type queryExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type Service struct {
	db        *pgxpool.Pool
	snowflake *core.Snowflake
	hub       *core.Hub
	mu        sync.RWMutex
}

func NewService(db *pgxpool.Pool, snowflake *core.Snowflake) (*Service, error) {
	return &Service{
		db:        db,
		snowflake: snowflake,
	}, nil
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
	request.Name = strings.TrimSpace(request.Name)
	request.Email = normalizeEmail(request.Email)

	s.mu.Lock()
	defer s.mu.Unlock()

	exists, err := s.memberExistsByEmail(request.Email)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if exists {
		writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(request.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "password_hash_failed", "unable to create member")
		return
	}

	block := []string{}
	read := make(map[string]string)
	member := core.MemberData{Id: s.snowflake.Next(), Name: request.Name, Email: request.Email, PasswordHash: string(hash), Block: block, Read: read}

	tx, err := s.db.Begin(context.Background())
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	defer tx.Rollback(context.Background())

	if err := s.storeMember(tx, member); err != nil {
		if isDuplicateEmailError(err) {
			writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	token, err := s.createToken(tx, member.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	member.Token = token

	if err := s.storeMember(tx, member); err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if err := tx.Commit(context.Background()); err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	responseMember, err := s.toMemberResponse(member, false)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
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
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || bcrypt.CompareHashAndPassword([]byte(member.PasswordHash), []byte(request.Password)) != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", errInvalidCredentials.Error())
		return
	}

	token := member.Token
	if core.SafeCompare(token, "") || !s.tokenBelongsToMember(token, s.snowflake.ToSnowflake(member.Id)) {
		token, err = s.createToken(s.db, member.Id)
		if err != nil {
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
		member.Token = token
		if err := s.storeMember(s.db, member); err != nil {
			s.deleteToken(token)
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
	}

	responseMember, err := s.toMemberResponse(member, false)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{
		"token":  token,
		"member": responseMember,
	}})
}

func (s *Service) RequireToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := memberToken(c)
		if !ok {
			writeError(c, http.StatusUnauthorized, "missing_token", "a member token is required")
			c.Abort()
			return
		}
		memberID, ok := s.validateToken(token)
		if !ok {
			writeError(c, http.StatusUnauthorized, "invalid_token", "token is invalid")
			c.Abort()
			return
		}
		c.Set("token", token)
		c.Set("member_id", memberID)
		c.Next()
	}
}

func (s *Service) Me(c *gin.Context) {
	memberID, _ := c.Get("member_id")
	member, err := s.memberByID(memberID.(string))
	if err == nil {
		responseMember, responseErr := s.toMemberResponse(member, false)
		if responseErr != nil {
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
		c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"member": responseMember}})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
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
	if request.Name == nil && request.Email == nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "username or email is required")
		return
	}

	memberID, _ := c.Get("member_id")
	s.mu.Lock()
	defer s.mu.Unlock()

	member, err := s.memberByID(memberID.(string))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
			return
		}
		writeError(c, http.StatusNotFound, "member_not_found", "member not found")
		return
	}

	if request.Name != nil {
		username := strings.TrimSpace(*request.Name)
		if core.SafeCompare(username, "") || len([]rune(username)) > maxnameLength {
			writeError(c, http.StatusBadRequest, "invalid_username", "username must be between 1 and 50 characters")
			return
		}
		member.Name = username
	}

	if request.Email != nil && normalizeEmail(*request.Email) != "" && normalizeEmail(*request.Email) != normalizeEmail(member.Email) {
		writeError(c, http.StatusForbidden, "email_change_not_allowed", "email cannot be changed")
		return
	}

	if err := s.storeMember(s.db, member); err != nil {
		if isDuplicateEmailError(err) {
			writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
			return
		}
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	responseMember, responseErr := s.toMemberResponse(member, false)
	if responseErr != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	memberWSResponse, responseErr := s.toMemberResponse(member, true)
	if responseErr != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}

	s.hub.BroadcastAll(core.WSEvent{
		Event:   core.WSEventEnum.ProfileUpdate,
		Payload: gin.H{"member": memberWSResponse},
	})
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
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
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

	newToken, err := s.createToken(s.db, member.Id)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	_, err = s.db.Exec(context.Background(),
		`UPDATE members SET password_hash = $1, token = $2 WHERE id = $3`,
		string(hash), newToken, s.snowflake.ToSnowflake(member.Id))
	if err != nil {
		s.deleteToken(newToken)
		writeError(c, http.StatusServiceUnavailable, "database_unavailable", errDatabaseUnavailable.Error())
		return
	}
	s.deleteToken(oldTokenValue)

	if s.hub != nil {
		s.hub.CloseMember(member.Id)
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) tokenBelongsToMember(token, memberID string) bool {
	memberID = s.snowflake.ToSnowflake(memberID)
	var id string
	err := s.db.QueryRow(context.Background(),
		`SELECT member_id FROM tokens WHERE token = $1`, token).Scan(&id)
	if err != nil {
		return false
	}
	return id == memberID
}

func (s *Service) toMemberResponse(member core.MemberData, limit bool) (memberResponse, error) {
	ctx := context.Background()
	memberID := s.snowflake.ToSnowflake(member.Id)

	rows, err := s.db.Query(ctx, `
		SELECT g.id, g.name, g.owner_id, g.created_at
		FROM groups g
		JOIN group_members gm ON gm.group_id = g.id
		WHERE gm.member_id = $1
		ORDER BY g.created_at ASC`, memberID)
	if err != nil {
		return memberResponse{}, err
	}
	defer rows.Close()

	groupData := make([]groupResponse, 0)
	for rows.Next() {
		var g groupRecord
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt); err != nil {
			return memberResponse{}, err
		}
		g.GroupID = g.ID
		groupData = append(groupData, toGroupResponse(s.snowflake, g))
	}
	if err := rows.Err(); err != nil {
		return memberResponse{}, err
	}

	if limit {
		return memberResponse{ID: s.snowflake.ToMemberID(member.Id), Name: member.Name, Block: []string{}, GroupRecords: make([]groupResponse, 0), Read: map[string]string{}}, nil
	}
	return memberResponse{ID: s.snowflake.ToMemberID(member.Id), Name: member.Name, Email: member.Email, GroupRecords: groupData, Block: member.Block, Read: member.Read}, nil
}

func (s *Service) getGroup(groupID string) (groupRecord, error) {
	groupID = s.snowflake.ToSnowflake(groupID)
	var g groupRecord
	err := s.db.QueryRow(context.Background(),
		`SELECT id, name, owner_id, created_at FROM groups WHERE id = $1`,
		groupID).Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt)
	if err != nil {
		return groupRecord{}, err
	}
	g.GroupID = g.ID
	g.OwnerID = s.snowflake.ToMemberID(g.OwnerID)
	return g, nil
}

func (s *Service) createToken(q queryExecer, memberID string) (string, error) {
	memberID = s.snowflake.ToSnowflake(memberID)
	token := newToken()
	_, err := q.Exec(context.Background(),
		`INSERT INTO tokens (token, member_id) VALUES ($1, $2)`, token, memberID)
	if err != nil {
		log.Printf("db token write failed member_id=%s: %v", memberID, err)
		return "", err
	}
	return token, nil
}

func (s *Service) deleteToken(token string) {
	_, err := s.db.Exec(context.Background(),
		`DELETE FROM tokens WHERE token = $1`, token)
	if err != nil {
		log.Printf("db token delete failed: %v", err)
	}
}

func (s *Service) storeMember(q queryExecer, member core.MemberData) error {
	member.Id = s.snowflake.ToSnowflake(member.Id)
	_, err := q.Exec(context.Background(), `
		INSERT INTO members (id, name, email, password_hash, token, block, read, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			password_hash = EXCLUDED.password_hash,
			token = EXCLUDED.token,
			block = EXCLUDED.block,
			read = EXCLUDED.read`,
		member.Id,
		member.Name,
		normalizeEmail(member.Email),
		member.PasswordHash,
		member.Token,
		jsonMarshal(member.Block),
		jsonMarshal(member.Read),
		s.snowflake.ToTime(member.Id))
	if err != nil {
		log.Printf("db member write failed id=%s: %v", member.Id, err)
		return err
	}
	return nil
}

func (s *Service) canCreateGroup(memberID string) (bool, error) {
	var count int
	err := s.db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM groups WHERE owner_id = $1`, memberID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count < maxOwnedGroups, nil
}

func (s *Service) addOwnedGroup(memberID, groupID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	canCreate, err := s.canCreateGroup(memberID)
	if err != nil {
		return err
	}
	if !canCreate {
		return errGroupLimitReached
	}
	_, err = s.db.Exec(context.Background(),
		`INSERT INTO groups (id, name, owner_id, created_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO NOTHING`,
		groupID, "", memberID, s.snowflake.ToTime(groupID))
	return err
}

func bindCredentials(c *gin.Context, request *credentialsRequest) bool {
	if err := c.ShouldBindJSON(request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "email must be valid and password must be at least 8 characters")
		return false
	}
	return true
}

func memberToken(c *gin.Context) (string, bool) {
	token := c.GetHeader("Authorization")
	if token == "" {
		return "", false
	}
	return token, true
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func newToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, core.Response{Status: "error", Code: code, Message: message})
}

func isDuplicateEmailError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "members_email_key"
}

func jsonMarshal(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(data)
}

func (s *Service) memberByID(memberID string) (core.MemberData, error) {
	memberID = s.snowflake.ToSnowflake(memberID)
	var m core.MemberData
	var block string
	var read string
	err := s.db.QueryRow(context.Background(), `
		SELECT id, name, email, password_hash, token, block, read
		FROM members WHERE id = $1`, memberID).
		Scan(&m.Id, &m.Name, &m.Email, &m.PasswordHash, &m.Token, &block, &read)
	if err != nil {
		return core.MemberData{}, err
	}
	m.Id = s.snowflake.ToMemberID(m.Id)
	if err := json.Unmarshal([]byte(block), &m.Block); err != nil {
		m.Block = []string{}
	}
	if err := json.Unmarshal([]byte(read), &m.Read); err != nil {
		m.Read = make(map[string]string)
	}
	return m, nil
}

func (s *Service) memberByEmail(email string) (core.MemberData, error) {
	email = normalizeEmail(email)
	var m core.MemberData
	var block string
	var read string
	err := s.db.QueryRow(context.Background(), `
		SELECT id, name, email, password_hash, token, block, read
		FROM members WHERE email = $1`, email).
		Scan(&m.Id, &m.Name, &m.Email, &m.PasswordHash, &m.Token, &block, &read)
	if err != nil {
		return core.MemberData{}, err
	}
	m.Id = s.snowflake.ToMemberID(m.Id)
	if err := json.Unmarshal([]byte(block), &m.Block); err != nil {
		m.Block = []string{}
	}
	if err := json.Unmarshal([]byte(read), &m.Read); err != nil {
		m.Read = make(map[string]string)
	}
	return m, nil
}

func (s *Service) memberExistsByEmail(email string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM members WHERE email = $1)`, normalizeEmail(email)).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Service) validateToken(token string) (string, bool) {
	var memberID string
	err := s.db.QueryRow(context.Background(),
		`SELECT member_id FROM tokens WHERE token = $1`, token).Scan(&memberID)
	if err != nil {
		return "", false
	}
	return s.snowflake.ToMemberID(memberID), true
}
