package database

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type PostgresDB struct {
	Pool *pgxpool.Pool
}

type RedisDB struct {
	Client *redis.Client
}

// NewPostgres creates a new PostgreSQL connection pool
func NewPostgres() (*PostgresDB, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgresql://ivan:Lost2409@localhost:5432/main-ste?sslmode=disable&timezone=Africa/Tunis"
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure connection pool
	config.MaxConns = 100
	config.MinConns = 10
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute * 30

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set timezone to Tunisia (UTC+1)
	if _, err := pool.Exec(ctx, "SET timezone = 'Africa/Tunis'"); err != nil {
		return nil, fmt.Errorf("failed to set timezone: %w", err)
	}

	return &PostgresDB{Pool: pool}, nil
}

// NewRedis creates a new Redis client
func NewRedis() (*RedisDB, error) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return &RedisDB{Client: client}, nil
}

// Close closes the database connections
func (db *PostgresDB) Close() {
	if db.Pool != nil {
		db.Pool.Close()
	}
}

func (db *RedisDB) Close() {
	if db.Client != nil {
		db.Client.Close()
	}
}
