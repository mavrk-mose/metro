package config

import (
	"os"
	"strconv"
)

type Config struct {
	ListenAddr  string
	DatabaseURL string
	RedisAddr   string
	NATSUrl     string
	WorkerCount int
	Environment string
}

func LoadConfig() *Config {
	return &Config{
		ListenAddr:  getEnv("LISTEN_ADDR", ":8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://localhost/seoul_metro"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		NATSUrl:     getEnv("NATS_URL", "nats://localhost:4222"),
		WorkerCount: getEnvInt("WORKER_COUNT", 16),
		Environment: getEnv("ENVIRONMENT", "development"),
	}
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	valStr := getEnv(key, "")
	if val, err := strconv.Atoi(valStr); err == nil {
		return val
	}
	return defaultVal
}
