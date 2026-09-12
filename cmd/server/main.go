package main

import (
	"context"
	"log"
	"os"
	"time"

	"willchat-backend/database"
	"willchat-backend/internal/api"
)

func main() {
	if err := database.LoadEnv(); err != nil {
		log.Fatal(err)
	}
	adr := os.Getenv("PORT")
	redisClient := database.RedisClient()
	redisContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := database.PingRedis(redisContext, redisClient); err != nil {
		log.Fatalf("redis connection failed addr=%s db=%d: %v", redisClient.Options().Addr, redisClient.Options().DB, err)
	}
	log.Printf("redis connected addr=%s db=%d", redisClient.Options().Addr, redisClient.Options().DB)

	router, err := api.Router(redisClient)
	if err != nil {
		log.Fatalf("redis user loading failed: %v", err)
	}
	if err := router.Run(":" + adr); err != nil {
		log.Fatal(err)
	}
}
