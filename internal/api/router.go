package api

import (
	"log"
	"time"

	"willchat-backend/internal/core"
	"willchat-backend/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var enableProxyChecker = false

func Router(redisClient *redis.Client) (*gin.Engine, error) {
	r := gin.Default()
	if enableProxyChecker {
		log.Print("Enabled proxy checker")
		r.Use(middleware.ProxyChecker())
	}

	r.GET("/health", middleware.RateLimitIP(5, 5*time.Second), Health)

	authService, err := NewService(redisClient, core.NewSnowflake(1))
	if err != nil {
		return nil, err
	}
	hub := core.NewHub()
	authService.SetHub(hub)
	chatService := NewChatService(redisClient, authService.snowflake, authService, hub)

	// Auth
	authRoutes := r.Group("/auth", middleware.RateLimitIP(3, 10*time.Second))
	authRoutes.POST("/register", authService.Register)
	authRoutes.POST("/login", authService.Login)

	// Token verification
	protectedRoutes := r.Group("/api", authService.RequireToken())
	protectedRoutes.GET("/me", middleware.RateLimitToken(10, 10*time.Second), middleware.RateLimitIP(25, 10*time.Second), authService.Me)
	protectedRoutes.PUT("/me", middleware.RateLimitToken(5, 10*time.Second), middleware.RateLimitIP(20, 10*time.Second), authService.UpdateProfile)

	// I dont know why I write this route
	// protectedRoutes.POST("/logout", middleware.RateLimit(5, 10*time.Second), authService.Logout)
	authRoutes.POST("/reset", middleware.RateLimitIP(10, 10*time.Second), authService.RequireToken(), middleware.RateLimitToken(5, 10*time.Second), authService.ResetPassword)

	// Group
	protected := r.Group("/api/groups", authService.RequireToken())
	protected.POST("", middleware.RateLimitToken(5, 10*time.Second), middleware.RateLimitIP(10, 10*time.Second), chatService.CreateGroup)
	protected.GET("/:group_id", middleware.RateLimitToken(7, 10*time.Second), middleware.RateLimitIP(20, 10*time.Second), chatService.GetGroup)
	protected.DELETE("/:group_id", middleware.RateLimitToken(5, 10*time.Second), middleware.RateLimitIP(10, 10*time.Second), chatService.DeleteGroup)
	protected.POST("/:group_id/members", middleware.RateLimitToken(5, 10*time.Second), middleware.RateLimitIP(10, 10*time.Second), chatService.JoinGroup)
	protected.DELETE("/:group_id/members", middleware.RateLimitToken(5, 10*time.Second), middleware.RateLimitIP(10, 10*time.Second), chatService.LeaveGroup)
	protected.GET("/:group_id/members/:member_id", middleware.RateLimitToken(7, 5*time.Second), middleware.RateLimitIP(14, 5*time.Second), chatService.GetMember)
	protected.POST("/:group_id/messages", middleware.RateLimitToken(10, 5*time.Second), middleware.RateLimitIP(25, 10*time.Second), chatService.SendMessage)
	protected.GET("/:group_id/messages", middleware.RateLimitToken(7, 5*time.Second), middleware.RateLimitIP(14, 5*time.Second), chatService.GetMessages)
	// protected.GET("/:group_id/message", middleware.RateLimit(5, 5*time.Second), chatService.GetMessage)

	// Websocket
	r.GET("/ws", middleware.RateLimitToken(10, 10*time.Second), middleware.RateLimitIP(25, 10*time.Second), chatService.WebSocket)

	return r, nil
}
