package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	PostgresDSN  string
	RedisAddr    string
	KafkaBrokers []string
	JWTSecret    string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Warn("failed to load .env file, using default values", "error", err)
	}

	cfg := &Config{
		PostgresDSN:  os.Getenv("POSTGRES_DSN"),
		RedisAddr:    os.Getenv("REDIS_ADDR"),
		KafkaBrokers: []string{os.Getenv("KAFKA_BROKER")},
		JWTSecret:    os.Getenv("JWT_SECRET"),
	}

	if cfg.PostgresDSN == "" {
		cfg.PostgresDSN = "host=localhost user=postgres password=postgres dbname=merch sslmode=disable"
	}
	if cfg.RedisAddr == "" {
		cfg.RedisAddr = "localhost:6379"
	}
	if len(cfg.KafkaBrokers) == 1 && cfg.KafkaBrokers[0] == "" {
		cfg.KafkaBrokers = []string{"localhost:9092"}
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "supersecret"
	}

	slog.Info("config loaded", "postgres_dsn", cfg.PostgresDSN, "redis_addr", cfg.RedisAddr, "kafka_brokers", cfg.KafkaBrokers)
	return cfg
}
