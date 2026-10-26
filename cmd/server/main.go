package main

import (
	"context"
	"log"
	"os"
	"time"

	"willchat-backend/data"
	"willchat-backend/internal/api"
)

func main() {
	if err := data.LoadEnv(); err != nil {
		log.Fatal(err)
	}
	adr := os.Getenv("PORT")
	if adr == "" {
		adr = "8080"
	}
	redisClient := data.RedisClient()
	redisContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := data.PingRedis(redisContext, redisClient); err != nil {
		log.Fatalf("redis connection failed addr=%s db=%d: %v", redisClient.Options().Addr, redisClient.Options().DB, err)
	}
	log.Printf("redis connected addr=%s db=%d", redisClient.Options().Addr, redisClient.Options().DB)

	router, err := api.Router(redisClient)
	if err != nil {
		log.Fatalf("redis member loading failed: %v", err)
	}
	if err := router.Run(":" + adr); err != nil {
		log.Fatal(err)
	}
}
