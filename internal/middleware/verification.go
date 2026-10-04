package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"willchat-backend/internal/core"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

func verifyHumanToken(ctx context.Context, token string, remoteIP string) bool {
	secret := os.Getenv("TURNSTILE_SECRET")
	if secret == "" {
		return false
	}

	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)

	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		turnstileVerifyURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return false
	}

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result core.TurnstileResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	return result.Success
}

func HumanChecker() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Human-Token")

		if token == "" {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				core.Response{
					Status:  "error",
					Code:    "human_verification_required",
					Message: "human verification required",
					Data:    gin.H{},
				},
			)
			return
		}

		if len(token) > 2048 {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				core.Response{
					Status:  "error",
					Code:    "invalid_human_verification",
					Message: "invalid human verification",
					Data:    gin.H{},
				},
			)
			return
		}

		ctx, cancel := context.WithTimeout(
			c.Request.Context(),
			5*time.Second,
		)
		defer cancel()

		if !verifyHumanToken(
			ctx,
			token,
			c.ClientIP(),
		) {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				core.Response{
					Status:  "error",
					Code:    "invalid_human_verification",
					Message: "invalid human verification",
					Data:    gin.H{},
				},
			)
			return
		}

		c.Next()
	}
}
