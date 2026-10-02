package data

import (
	"context"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"

	"willchat-backend/internal/core"
)

// RedisClient
func RedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     core.XorDecipher(envOrDefault("REDIS_ADDR", "595a5654595d5a46410f0306020c"), 53),
		Password: core.XorDecipher(os.Getenv("REDIS_PASSWORD"), 122),
		DB:       envIntOrDefault("REDIS_DB", 0),

		// TLSConfig: &tls.Config{},
	})
}

func PingRedis(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
