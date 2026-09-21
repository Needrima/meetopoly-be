package verification

import (
	"context"
	"time"
)

// Record is a pending email verification code.
type Record struct {
	ID        string
	Email     string
	CodeHash  string
	ExpiresAt time.Time
	Attempts  int
	CreatedAt time.Time
}

// Repository persists email verification codes.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Upsert(ctx context.Context, email, codeHash string, expiresAt time.Time) error
	FindByEmail(ctx context.Context, email string) (*Record, error)
	IncrementAttempts(ctx context.Context, email string) error
	DeleteByEmail(ctx context.Context, email string) error
}
