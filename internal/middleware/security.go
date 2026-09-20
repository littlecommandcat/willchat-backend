package middleware

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"

	"willchat-backend/internal/core"
)

func ProxyChecker() gin.HandlerFunc {
	return func(c *gin.Context) {
		var mu sync.Mutex
		mu.Lock()
		remoteAddr := c.Request.RemoteAddr
		forwardedFor := c.GetHeader("X-Forwarded-For")
		log.Printf("Get remoteAddr: %s  forwardedFor: %s", remoteAddr, forwardedFor)
		if core.SafeCompare(remoteAddr, forwardedFor) {
			mu.Unlock()
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				core.Response{
					Status:  "error",
					Code:    "forbidden_forwarded",
					Message: "Forbidden forwarded",
					Data:    gin.H{"message": "Forbidden forwarded"},
				},
			)
			return
		}
		mu.Unlock()
		c.Next()
	}
}
