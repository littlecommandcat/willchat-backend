package middleware

import (
	"net/http"
	// "strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"willchat-backend/internal/core"
)

func RateLimitToken(max int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	requestsToken := make(map[string][]time.Time)
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")

		token := auth
		// if strings.HasPrefix(auth, "") {
		// 	token = strings.TrimPrefix(auth, "")
		// }
		now := time.Now()
		mu.Lock()

		// Token ratelimit
		timesToken := requestsToken[token]
		recentTokenTimes := timesToken[:0]

		for _, t := range timesToken {
			if now.Sub(t) < window {
				recentTokenTimes = append(recentTokenTimes, t)
			}
		}

		if len(recentTokenTimes) >= max {
			mu.Unlock()
			c.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				core.Response{
					Status:  "error",
					Code:    "too_many_requests",
					Message: "Too many requests",
					Data:    gin.H{},
				},
			)
			return
		}
		recentTokenTimes = append(recentTokenTimes, now)
		requestsToken[token] = recentTokenTimes

		mu.Unlock()
		c.Next()
	}
}

func RateLimitIP(max int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	requestsIp := make(map[string][]time.Time)
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		mu.Lock()

		// Ip ratelimit
		timesIp := requestsIp[ip]
		recentIpTimes := timesIp[:0]

		for _, t := range timesIp {
			if now.Sub(t) < window {
				recentIpTimes = append(recentIpTimes, t)
			}
		}

		// Extra code
		if len(recentIpTimes) >= (max * 3) {
			mu.Unlock()
			c.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				core.Response{
					Status:  "error",
					Code:    "too_many_requests",
					Message: "Global rate limit",
					Data:    gin.H{},
				},
			)
			return
		}
		recentIpTimes = append(recentIpTimes, now)
		requestsIp[ip] = recentIpTimes

		mu.Unlock()
		c.Next()
	}
}
