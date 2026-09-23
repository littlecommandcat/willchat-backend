package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const verificationTTL = 10 * time.Minute

var outlookHTTP = &http.Client{Timeout: 10 * time.Second}
var outlookMu sync.Mutex
var outlookAccessToken string
var outlookTokenExpiry time.Time
var outlookRefreshToken string

func verificationKey(email string) string         { return "verification:" + email }
func verificationCooldownKey(email string) string { return "verification-cooldown:" + email }
func verificationAttemptsKey(email string) string { return "verification-attempts:" + email }

func (s *Service) SendVerification(c *gin.Context) {
	var request verificationRequest
	if c.ShouldBindJSON(&request) != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "email must be valid")
		return
	}
	email := normalizeEmail(request.Email)
	if _, err := s.memberByEmail(email); err == nil {
		writeError(c, http.StatusConflict, "email_already_registered", "email is already registered")
		return
	} else if !errors.Is(err, redis.Nil) {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	ctx := c.Request.Context()
	ok, err := s.redis.SetNX(ctx, verificationCooldownKey(email), "1", time.Minute).Result()
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		return
	}
	if !ok {
		writeError(c, http.StatusTooManyRequests, "too_many_requests", "wait one minute before requesting another code")
		return
	}
	defer func() {
		if !ok {
			_ = s.redis.Del(context.Background(), verificationCooldownKey(email)).Err()
		}
	}()

	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "verification_failed", "unable to generate code")
		ok = false
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "verification_failed", "unable to generate code")
		ok = false
		return
	}
	if err = s.redis.Set(ctx, verificationKey(email), hash, verificationTTL).Err(); err != nil {
		writeError(c, http.StatusServiceUnavailable, "redis_unavailable", errRedisUnavailable.Error())
		ok = false
		return
	}
	_ = s.redis.Del(ctx, verificationAttemptsKey(email)).Err()
	if err = sendOutlookMail(ctx, email, code); err != nil {
		log.Printf("send verification mail failed: %v", err)
		_ = s.redis.Del(context.Background(), verificationKey(email)).Err()
		writeError(c, http.StatusServiceUnavailable, "mail_unavailable", "unable to send verification email")
		ok = false
		return
	}
	c.JSON(http.StatusOK, core.Response{Status: "ok", Code: "success", Data: gin.H{"expires_in": int(verificationTTL.Seconds())}})
}

func (s *Service) consumeVerification(ctx context.Context, email, code string) (bool, error) {
	if len(code) != 6 {
		return false, nil
	}
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			return false, nil
		}
	}
	attempts, err := s.redis.Incr(ctx, verificationAttemptsKey(email)).Result()
	if err != nil {
		return false, err
	}
	if attempts == 1 {
		if err := s.redis.Expire(ctx, verificationAttemptsKey(email), verificationTTL).Err(); err != nil {
			return false, err
		}
	}
	if attempts > 5 {
		return false, nil
	}
	hash, err := s.redis.Get(ctx, verificationKey(email)).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(code)) != nil {
		return false, nil
	}
	// GETDEL is atomic across server instances; only one registration can use the code.
	used, err := s.redis.GetDel(ctx, verificationKey(email)).Bytes()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(used) == string(hash), nil
}

func sendOutlookMail(ctx context.Context, email, code string) error {
	token, err := outlookToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"subject":      "WillChat verification code",
		"body":         map[string]string{"contentType": "Text", "content": "Your WillChat verification code is " + code + ". It expires in 10 minutes."},
		"toRecipients": []any{map[string]any{"emailAddress": map[string]string{"address": email}}},
	}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://graph.microsoft.com/v1.0/me/sendMail", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := outlookHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Graph sendMail returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func outlookToken(ctx context.Context) (string, error) {
	outlookMu.Lock()
	defer outlookMu.Unlock()
	if outlookAccessToken != "" && time.Now().Before(outlookTokenExpiry) {
		return outlookAccessToken, nil
	}
	clientID := os.Getenv("OUTLOOK_CLIENT_ID")
	if outlookRefreshToken == "" {
		outlookRefreshToken = os.Getenv("OUTLOOK_REFRESH_TOKEN")
	}
	if clientID == "" || outlookRefreshToken == "" {
		return "", errors.New("OUTLOOK_CLIENT_ID and OUTLOOK_REFRESH_TOKEN are required")
	}
	form := url.Values{"client_id": {clientID}, "grant_type": {"refresh_token"}, "refresh_token": {outlookRefreshToken}, "scope": {"offline_access Mail.Send"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://login.microsoftonline.com/consumers/oauth2/v2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := outlookHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Microsoft token endpoint returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.AccessToken == "" {
		return "", errors.New("Microsoft returned no access token")
	}
	if result.RefreshToken != "" {
		outlookRefreshToken = result.RefreshToken
	}
	outlookAccessToken = result.AccessToken
	outlookTokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return outlookAccessToken, nil
}
