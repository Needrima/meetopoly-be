package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds process configuration from environment (no Docker).
type Config struct {
	HTTPAddr      string
	MongoURI      string
	MongoDatabase string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	PingTimeout   time.Duration
	Version       string
	LogFile       string
	LogLevel      string
	LogFormat     string
}

func Load() Config {
	return Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		MongoURI:      getenv("MONGO_URI", "mongodb://127.0.0.1:27017"),
		MongoDatabase: getenv("MONGO_DATABASE", "meetopoly"),
		RedisAddr:     getenv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
		PingTimeout:   time.Duration(getenvInt("PING_TIMEOUT_MS", 2000)) * time.Millisecond,
		Version:       getenv("APP_VERSION", "0.0.1-phase0"),
		LogFile:       getenv("LOG_FILE", "app.log"),
		LogLevel:      getenv("LOG_LEVEL", "info"),
		LogFormat:     getenv("LOG_FORMAT", "text"),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
