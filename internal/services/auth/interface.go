package auth

import (
	"context"

	usersvc "meetopoly-be/internal/services/user"
)

// Service owns signup and login flows.
type Service interface {
	StartSignup(ctx context.Context, email string) error
	VerifyEmail(ctx context.Context, email, code string) (signupToken string, err error)
	SetPassword(ctx context.Context, signupToken, password string) error
	CompleteProfile(ctx context.Context, signupToken, username, country string) (sessionToken string, profile *usersvc.Profile, err error)
	Login(ctx context.Context, email, password string) (sessionToken string, profile *usersvc.Profile, err error)
	Logout(ctx context.Context, sessionToken string) error
	ResolveSession(ctx context.Context, token string) (userID string, err error)
	ResolveSignup(ctx context.Context, token string) (userID string, email string, err error)
}
