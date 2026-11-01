package data

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	host := envOrDefault("POSTGRES_HOST", "localhost")
	port := envOrDefault("POSTGRES_PORT", "5432")
	user := envOrDefault("POSTGRES_USER", "willchat")
	password := envOrDefault("POSTGRES_PASSWORD", "willchat")
	database := envOrDefault("POSTGRES_DB", "willchat")
	sslmode := envOrDefault("POSTGRES_SSLMODE", "disable")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, database, sslmode,
	)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping failed: %w", err)
	}

	return pool, nil
}

// migrate function
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	const schema = `
CREATE TABLE IF NOT EXISTS members (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    email       TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    token       TEXT NOT NULL DEFAULT '',
    block       JSONB NOT NULL DEFAULT '[]'::jsonb,
    read        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    owner_id    TEXT NOT NULL REFERENCES members(id) ON DELETE RESTRICT,
    created_at  BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id    TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    member_id   TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, member_id)
);

CREATE INDEX IF NOT EXISTS idx_group_members_member ON group_members(member_id);

CREATE TABLE IF NOT EXISTS attachments (
    id          TEXT PRIMARY KEY,
    mime_type   TEXT NOT NULL,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    size        INTEGER NOT NULL DEFAULT 0,
    data        BYTEA NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    group_id        TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    author_id       TEXT NOT NULL REFERENCES members(id),
    content         TEXT NOT NULL,
    status          INTEGER NOT NULL DEFAULT 0,
    type            INTEGER NOT NULL DEFAULT 0,
    reply_to_id     TEXT,
    reply_to_author_id TEXT,
    reply_to_content TEXT,
    reply_to_type   INTEGER,
    created_at      BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_group_created ON messages(group_id, created_at);

CREATE TABLE IF NOT EXISTS message_attachments (
    message_id    TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    attachment_id TEXT NOT NULL REFERENCES attachments(id) ON DELETE CASCADE,
    PRIMARY KEY (message_id, attachment_id)
);

CREATE TABLE IF NOT EXISTS tokens (
    token     TEXT PRIMARY KEY,
    member_id TEXT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`
	_, err := pool.Exec(ctx, schema)
	if err != nil {
		return fmt.Errorf("postgres migrate failed: %w", err)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// init
func init() {
	_ = os.Getenv("POSTGRES_HOST")
}
