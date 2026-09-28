package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dev-nathaniel/Adroit-platform/pkg/config"
)

// Config holds PostgreSQL connection parameters
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// LoadConfig loads database settings from environment variables
func LoadConfig() Config {
	return Config{
		Host:            config.GetEnv("DB_HOST", "localhost"),
		Port:            config.GetEnvInt("DB_PORT", 5432),
		User:            config.GetEnv("DB_USER", "adroit"),
		Password:        config.GetEnv("DB_PASSWORD", "adroit_secret"),
		Database:        config.GetEnv("DB_NAME", "adroit_platform"),
		SSLMode:         config.GetEnv("DB_SSLMODE", "disable"),
		MaxOpenConns:    config.GetEnvInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:    config.GetEnvInt("DB_MAX_IDLE_CONNS", 10),
		ConnMaxLifetime: config.GetEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
	}
}

// DSN returns the PostgreSQL connection string
func (c Config) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode,
	)
}

// Connect opens and verifies a pooled connection to PostgreSQL
func Connect(ctx context.Context, cfg Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// RunMigrations executes an SQL migration script against the database
func RunMigrations(ctx context.Context, db *sql.DB, sqlScript string) error {
	_, err := db.ExecContext(ctx, sqlScript)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}
