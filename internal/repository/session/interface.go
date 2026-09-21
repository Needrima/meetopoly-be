package session

import (
	"context"
	"time"
)

// Kind distinguishes full sessions from short-lived signup tokens.
type Kind string

const (
	KindSession Kind = "session"
	KindSignup  Kind = "signup"
)

// TokenData is stored under an opaque Redis token.
type TokenData struct {
	UserID string
	Email  string
	Kind   Kind
}

// Repository stores opaque auth tokens in Redis.
type Repository interface {
	Create(ctx context.Context, token string, data TokenData, ttl time.Duration) error
	Get(ctx context.Context, token string) (*TokenData, error)
	Delete(ctx context.Context, token string) error
}
