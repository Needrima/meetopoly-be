package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
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
	LogFormat     string

	SignupTokenTTL      time.Duration
	VerificationCodeTTL time.Duration
	SMTPHost            string
	SMTPPort            int
	SMTPUser            string
	SMTPPass            string
	SMTPFrom            string
}

// Load reads optional `.env` via godotenv, then environment variables.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
		MongoURI:      getenv("MONGO_URI", "mongodb://127.0.0.1:27017"),
		MongoDatabase: getenv("MONGO_DATABASE", "meetopoly"),
		RedisAddr:     getenv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
		PingTimeout:   time.Duration(getenvInt("PING_TIMEOUT_MS", 2000)) * time.Millisecond,
		Version:       getenv("APP_VERSION", "0.3.0-phase3"),
		LogFile:       getenv("LOG_FILE", "app.log"),
		LogFormat:     getenv("LOG_FORMAT", "text"),

		SignupTokenTTL:      time.Duration(getenvInt("SIGNUP_TOKEN_TTL_MINUTES", 30)) * time.Minute,
		VerificationCodeTTL: time.Duration(getenvInt("VERIFICATION_CODE_TTL_MINUTES", 2)) * time.Minute,

		SMTPHost: getenv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort: getenvInt("SMTP_PORT", 587),
		SMTPUser: getenv("SMTP_USER", ""),
		SMTPPass: getenv("SMTP_PASS", ""),
		SMTPFrom: getenv("SMTP_FROM", ""),
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
