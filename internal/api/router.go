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

	r.GET("/health", middleware.RateLimit(5, 5*time.Second), Health)

	authService, err := NewService(redisClient, core.NewSnowflake(1))
	if err != nil {
		return nil, err
	}
	hub := core.NewHub()
	authService.SetHub(hub)
	chatService := NewChatService(redisClient, authService.snowflake, authService, hub)

	// Auth
	authRoutes := r.Group("/auth", middleware.RateLimit(3, 10*time.Second))
	authRoutes.POST("/verification-code", authService.SendVerification)
	authRoutes.POST("/register", authService.Register)
	authRoutes.POST("/login", authService.Login)

	// Token verification
	protectedRoutes := r.Group("/auth", authService.RequireToken())
	protectedRoutes.GET("/me", middleware.RateLimit(10, 10*time.Second), authService.Me)
	protectedRoutes.PUT("/me", middleware.RateLimit(5, 10*time.Second), authService.UpdateProfile)

	// Auth but verify token

	// I dont know why I write this route
	// protectedRoutes.POST("/logout", middleware.RateLimit(5, 10*time.Second), authService.Logout)
	protectedRoutes.POST("/reset", middleware.RateLimit(5, 10*time.Second), authService.ResetPassword)

	// Group
	protected := r.Group("/groups", authService.RequireToken())
	protected.POST("", middleware.RateLimit(7, 10*time.Second), chatService.CreateGroup)
	protected.GET("/:group_id", middleware.RateLimit(7, 10*time.Second), chatService.GetGroup)
	protected.DELETE("/:group_id", middleware.RateLimit(5, 10*time.Second), chatService.DeleteGroup)
	protected.POST("/:group_id/members", middleware.RateLimit(5, 10*time.Second), chatService.JoinGroup)
	protected.DELETE("/:group_id/members", middleware.RateLimit(5, 10*time.Second), chatService.LeaveGroup)
	protected.GET("/:group_id/members/:member_id", middleware.RateLimit(10, 5*time.Second), chatService.GetMember)
	protected.POST("/:group_id/messages", middleware.RateLimit(10, 10*time.Second), chatService.SendMessage)
	protected.GET("/:group_id/messages", middleware.RateLimit(10, 5*time.Second), chatService.GetMessages)
	// protected.GET("/:group_id/message", middleware.RateLimit(5, 5*time.Second), chatService.GetMessage)

	// Websocket
	r.GET("/ws", middleware.RateLimit(10, 10*time.Second), chatService.WebSocket)

	// Web page
	//r.Static("/app", "./public")

	return r, nil
}
