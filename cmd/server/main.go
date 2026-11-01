package main

import (
	"context"
	"encoding/json"
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
	serverVersion := os.Getenv("SERVER_VERSION")
	if serverVersion == "" {
		serverVersion = "1.0.0"
	}
	data.LatestServerVersion = serverVersion
	log.Printf("server version: %s", serverVersion)
	// app version
	pythonAppVersion := data.GetEnvPointer("APP_PYTHON_VERSION")
	rustAppVersion := data.GetEnvPointer("APP_RUST_VERSION")
	cppAppVersion := data.GetEnvPointer("APP_CPP_VERSION")
	data.LatestAppVersion = data.AppLatestVersion{
		Python: pythonAppVersion,
		Rust:   rustAppVersion,
		Cpp:    cppAppVersion,
	}
	versionData, err := json.Marshal(data.LatestAppVersion)
	if err != nil {
		log.Printf("failed to load app version: %v", err)
	}
	versionString := string(versionData)
	log.Printf("app version: %s", versionString)
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
