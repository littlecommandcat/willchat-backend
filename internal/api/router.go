package api

import (
	"willchat-backend/internal/core"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func Router(redisClient *redis.Client) (*gin.Engine, error) {
	r := gin.Default()

	r.Any("/health", Health)

	authService, err := NewService(redisClient, core.NewSnowflake(1))
	if err != nil {
		return nil, err
	}
	chatService := NewChatService(redisClient, authService.snowflake, authService)
	authRoutes := r.Group("/auth")
	authRoutes.POST("/register", authService.Register)
	authRoutes.POST("/login", authService.Login)

	// Token verification
	protected := r.Group("", authService.RequireToken())
	protected.GET("/auth/me", authService.Me)
	protected.POST("/auth/logout", authService.Logout)
	protected.POST("/groups", chatService.CreateGroup)
	protected.GET("/groups/:group_id", chatService.GetGroup)
	protected.POST("/groups/:group_id/members", chatService.JoinGroup)
	protected.DELETE("/groups/:group_id/members", chatService.LeaveGroup)
	protected.POST("/groups/:group_id/messages", chatService.SendMessage)
	r.GET("/ws/groups/:group_id", chatService.WebSocket)

	return r, nil
}
