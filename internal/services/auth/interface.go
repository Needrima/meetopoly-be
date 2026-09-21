package auth

import (
	"context"

	usersvc "meetopoly-be/internal/services/user"
)

// LoginResult is returned by Login — either a full session or a resume-to-profile signup token.
type LoginResult struct {
	NeedsProfile bool
	SessionToken string
	SignupToken  string
	Profile      *usersvc.Profile
}

// SignupStatus describes durable signup progress for a signup Bearer token.
// Resume only applies when PasswordSet is true (OTP-only progress is discarded).
type SignupStatus struct {
	Email           string
	PasswordSet     bool
	ProfileComplete bool
}

// Service owns signup, login, and password-reset flows.
type Service interface {
	StartSignup(ctx context.Context, email string) error
	VerifyEmail(ctx context.Context, email, code string) (signupToken string, err error)
	SetPassword(ctx context.Context, signupToken, password string) error
	CompleteProfile(ctx context.Context, signupToken, username, country string) (sessionToken string, profile *usersvc.Profile, err error)
	Login(ctx context.Context, email, password string) (*LoginResult, error)
	Logout(ctx context.Context, sessionToken string) error
	SignupStatus(ctx context.Context, signupToken string) (*SignupStatus, error)
	// StartPasswordReset sends a code only for fully registered users. sent is false when no mail went out.
	StartPasswordReset(ctx context.Context, email string) (sent bool, err error)
	VerifyPasswordReset(ctx context.Context, email, code string) (resetToken string, err error)
	ResetPassword(ctx context.Context, resetToken, password string) error
	ResolveSession(ctx context.Context, token string) (userID string, err error)
	ResolveSignup(ctx context.Context, token string) (userID string, email string, err error)
}
