package user

import (
	"context"
	"time"
)

// User is the persisted account document.
type User struct {
	ID            string
	Email         string
	PasswordHash  string
	Username      string
	Country       string
	EmailVerified bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProfileComplete reports whether signup profile fields are set.
func (u User) ProfileComplete() bool {
	return u.Username != "" && u.Country != ""
}

// Repository persists users in MongoDB.
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Create(ctx context.Context, u *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, u *User) error
}
