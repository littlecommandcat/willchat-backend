package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"willchat-backend/internal/core"
)

func RateLimit(max int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	requestsIp := make(map[string][]time.Time)
	requestsToken := make(map[string][]time.Time)
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")

		token := "Unauth"
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}

		ip := c.ClientIP()
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
					Data:    gin.H{"message": "Too many requests"},
				},
			)
			return
		}
		recentTokenTimes = append(recentTokenTimes, now)
		requestsToken[token] = recentTokenTimes

		// Ip ratelimit
		timesIp := requestsIp[ip]
		recentIpTimes := timesIp[:0]

		for _, t := range timesIp {
			if now.Sub(t) < window {
				recentIpTimes = append(recentIpTimes, t)
			}
		}

		if len(recentIpTimes) >= max {
			mu.Unlock()
			c.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				core.Response{
					Status:  "error",
					Code:    "too_many_requests",
					Message: "Too many requests",
					Data:    gin.H{"message": "Too many requests"},
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
