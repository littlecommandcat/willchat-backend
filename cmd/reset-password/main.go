package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	"willchat-backend/database"

	"golang.org/x/crypto/bcrypt"
)

type redisUserRecord struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	PasswordHash string   `json:"password_hash"`
	GroupIDs     []string `json:"group_ids"`
}

func main() {
	email := flag.String("email", "", "user email")
	password := flag.String("password", "", "new password")
	deduplicate := flag.Bool("deduplicate", false, "keep only the record with the greatest user ID")
	flag.Parse()

	if strings.TrimSpace(*email) == "" || len(*password) < 8 {
		log.Fatal("both -email and -password are required; password must be at least 8 characters")
	}

	if err := database.LoadEnv(); err != nil {
		log.Fatal(err)
	}

	client := database.RedisClient()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connection failed addr=%s db=%d: %v", client.Options().Addr, client.Options().DB, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("password hash failed: %v", err)
	}

	targetEmail := strings.ToLower(strings.TrimSpace(*email))
	type match struct {
		key    string
		record redisUserRecord
	}
	var matches []match
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(ctx, cursor, "user:*", 0).Result()
		if err != nil {
			log.Fatalf("redis user scan failed: %v", err)
		}

		for _, key := range keys {
			value, err := client.Get(ctx, key).Result()
			if err != nil {
				log.Fatalf("redis user read failed key=%s: %v", key, err)
			}

			var record redisUserRecord
			if err := json.Unmarshal([]byte(value), &record); err != nil {
				log.Fatalf("redis user decode failed key=%s: %v", key, err)
			}
			if strings.ToLower(strings.TrimSpace(record.Email)) != targetEmail {
				continue
			}
			matches = append(matches, match{key: key, record: record})
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if len(matches) == 0 {
		log.Fatalf("user not found email=%s", targetEmail)
	}
	if *deduplicate && len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].record.ID > matches[j].record.ID })
		for _, duplicate := range matches[1:] {
			if err := client.Del(ctx, duplicate.key).Err(); err != nil {
				log.Fatalf("redis duplicate user delete failed key=%s: %v", duplicate.key, err)
			}
			fmt.Printf("duplicate user deleted email=%s key=%s user_id=%s\n", targetEmail, duplicate.key, duplicate.record.ID)
		}
		matches = matches[:1]
	}

	updated := 0
	for _, found := range matches {
		record := found.record

		record.PasswordHash = string(hash)
		payload, err := json.Marshal(record)
		if err != nil {
			log.Fatalf("redis user encode failed key=%s: %v", found.key, err)
		}
		if err := client.Set(ctx, found.key, payload, 0).Err(); err != nil {
			log.Fatalf("redis user write failed key=%s: %v", found.key, err)
		}
		stored, err := client.Get(ctx, found.key).Result()
		if err != nil {
			log.Fatalf("redis user read-back failed key=%s: %v", found.key, err)
		}
		if stored != string(payload) {
			log.Fatalf("redis user read-back mismatch key=%s", found.key)
		}
		fmt.Printf("password reset successfully email=%s key=%s user_id=%s\n", targetEmail, found.key, record.ID)
		updated++
	}
	fmt.Printf("password reset completed email=%s records=%d\n", targetEmail, updated)
}
