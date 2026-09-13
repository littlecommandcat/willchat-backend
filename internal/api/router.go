package api

import (
	"time"

	"willchat-backend/internal/core"
	"willchat-backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func Router(redisClient *redis.Client) (*gin.Engine, error) {
	r := gin.Default()

	r.GET("/health", Health)

	authService, err := NewService(redisClient, core.NewSnowflake(1))
	if err != nil {
		return nil, err
	}
	hub := core.NewHub()
	authService.SetHub(hub)
	chatService := NewChatService(redisClient, authService.snowflake, authService, hub)
	authRoutes := r.Group("/auth")
	authRoutes.POST("/register", authService.Register)
	authRoutes.POST("/login", authService.Login)

	// Token verification
	protected := r.Group("", authService.RequireToken())
	protected.GET("/auth/me", middleware.RateLimit(10, 10*time.Second), authService.Me)
	protected.PUT("/auth/me", middleware.RateLimit(10, 5*time.Second), authService.UpdateProfile)
	protected.POST("/auth/logout", middleware.RateLimit(10, 5*time.Second), authService.Logout)
	protected.POST("/groups", middleware.RateLimit(10, 7*time.Second), chatService.CreateGroup)
	protected.GET("/groups/:group_id", middleware.RateLimit(10, 7*time.Second), chatService.GetGroup)
	protected.POST("/groups/:group_id/members", middleware.RateLimit(10, 5*time.Second), chatService.JoinGroup)
	protected.DELETE("/groups/:group_id/members", middleware.RateLimit(10, 5*time.Second), chatService.LeaveGroup)
	protected.POST("/groups/:group_id/messages", middleware.RateLimit(10, 10*time.Second), chatService.SendMessage)
	r.GET("/ws/groups/:group_id", middleware.RateLimit(10, 10*time.Second), chatService.WebSocket)

	return r, nil
}
