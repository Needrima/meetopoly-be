package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	stdmail "net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	sessionrepo "meetopoly-be/internal/repository/session"
	userrepo "meetopoly-be/internal/repository/user"
	verificationrepo "meetopoly-be/internal/repository/verification"
	mailsvc "meetopoly-be/internal/services/mail"
	usersvc "meetopoly-be/internal/services/user"
)

var (
	// ErrInvalidEmail is returned for malformed emails.
	ErrInvalidEmail = errors.New("invalid email")
	// ErrInvalidCode is returned for wrong/expired verification codes.
	ErrInvalidCode = errors.New("invalid verification code")
	// ErrWeakPassword is returned when password policy fails.
	ErrWeakPassword = errors.New("weak password")
	// ErrInvalidUsername is returned for bad usernames.
	ErrInvalidUsername = errors.New("invalid username")
	// ErrInvalidCountry is returned for bad country codes.
	ErrInvalidCountry = errors.New("invalid country")
	// ErrUnauthorized is returned for missing/invalid tokens or credentials.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrUsernameTaken is returned when username is already used.
	ErrUsernameTaken = errors.New("username taken")
	// ErrIncompleteSignup is returned when login is attempted before profile is done.
	ErrIncompleteSignup = errors.New("incomplete signup")
	// ErrEmailUnverified is returned when login is attempted before verify.
	ErrEmailUnverified = errors.New("email unverified")
	// ErrTooManyAttempts is returned after too many wrong codes.
	ErrTooManyAttempts = errors.New("too many verification attempts")
)

var (
	usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)
	countryRE  = regexp.MustCompile(`^[A-Z]{2}$`)
)

const (
	bcryptCost        = 12
	maxVerifyAttempts = 8
)

// Config holds TTLs for short-lived signup tokens and email codes.
// Login sessions have no TTL — they persist until logout.
type Config struct {
	SignupTokenTTL      time.Duration
	VerificationCodeTTL time.Duration
}

type service struct {
	users    userrepo.Repository
	codes    verificationrepo.Repository
	sessions sessionrepo.Repository
	mailer   mailsvc.Service
	cfg      Config
}

// New builds an auth Service.
func New(
	users userrepo.Repository,
	codes verificationrepo.Repository,
	sessions sessionrepo.Repository,
	mailer mailsvc.Service,
	cfg Config,
) Service {
	return &service{
		users:    users,
		codes:    codes,
		sessions: sessions,
		mailer:   mailer,
		cfg:      cfg,
	}
}

func (s *service) StartSignup(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return ErrInvalidEmail
	}

	existing, err := s.users.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, userrepo.ErrNotFound) {
		return fmt.Errorf("start signup find user: %w", err)
	}
	if existing != nil && existing.ProfileComplete() && existing.PasswordHash != "" {
		// Anti-enumeration: pretend success for fully registered accounts.
		return nil
	}

	if existing == nil {
		u := &userrepo.User{
			Email:         email,
			EmailVerified: false,
		}
		if err := s.users.Create(ctx, u); err != nil {
			if errors.Is(err, userrepo.ErrDuplicate) {
				// Race: treat as success path and continue to send code.
			} else {
				return fmt.Errorf("start signup create user: %w", err)
			}
		}
	}

	code, err := randomDigits(6)
	if err != nil {
		return fmt.Errorf("start signup code: %w", err)
	}
	codeHash := hashCode(code)
	expires := time.Now().UTC().Add(s.cfg.VerificationCodeTTL)
	if err := s.codes.Upsert(ctx, email, codeHash, expires); err != nil {
		return fmt.Errorf("start signup store code: %w", err)
	}

	body := fmt.Sprintf(
		"Your Meetopoly verification code is: %s\n\nIt expires in %d minutes.\n",
		code,
		int(s.cfg.VerificationCodeTTL.Minutes()),
	)
	if err := s.mailer.Send(ctx, mailsvc.Message{
		To:      email,
		Subject: "Meetopoly verification code",
		Body:    body,
	}); err != nil {
		return fmt.Errorf("start signup mail: %w", err)
	}
	return nil
}

func (s *service) VerifyEmail(ctx context.Context, email, code string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", ErrInvalidEmail
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 || !allDigits(code) {
		return "", ErrInvalidCode
	}

	rec, err := s.codes.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, verificationrepo.ErrNotFound) {
			return "", ErrInvalidCode
		}
		return "", fmt.Errorf("verify find code: %w", err)
	}
	if rec.Attempts >= maxVerifyAttempts {
		return "", ErrTooManyAttempts
	}
	if time.Now().UTC().After(rec.ExpiresAt) {
		return "", ErrInvalidCode
	}
	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(hashCode(code))) != 1 {
		_ = s.codes.IncrementAttempts(ctx, email)
		return "", ErrInvalidCode
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return "", ErrInvalidCode
		}
		return "", fmt.Errorf("verify find user: %w", err)
	}
	u.EmailVerified = true
	if err := s.users.Update(ctx, u); err != nil {
		return "", fmt.Errorf("verify update user: %w", err)
	}
	if err := s.codes.DeleteByEmail(ctx, email); err != nil {
		return "", fmt.Errorf("verify delete code: %w", err)
	}

	token, err := s.issueSignupToken(ctx, u)
	if err != nil {
		return "", fmt.Errorf("verify token: %w", err)
	}
	return token, nil
}

func (s *service) SetPassword(ctx context.Context, signupToken, password string) error {
	data, err := s.requireSignup(ctx, signupToken)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("set password hash: %w", err)
	}
	u, err := s.users.FindByID(ctx, data.UserID)
	if err != nil {
		return fmt.Errorf("set password find user: %w", err)
	}
	if !u.EmailVerified {
		return ErrEmailUnverified
	}
	u.PasswordHash = string(hash)
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("set password update: %w", err)
	}
	return nil
}

func (s *service) CompleteProfile(
	ctx context.Context,
	signupToken, username, country string,
) (string, *usersvc.Profile, error) {
	data, err := s.requireSignup(ctx, signupToken)
	if err != nil {
		return "", nil, err
	}
	username = strings.TrimSpace(username)
	country = strings.ToUpper(strings.TrimSpace(country))
	if !usernameRE.MatchString(username) {
		return "", nil, ErrInvalidUsername
	}
	if !countryRE.MatchString(country) {
		return "", nil, ErrInvalidCountry
	}

	u, err := s.users.FindByID(ctx, data.UserID)
	if err != nil {
		return "", nil, fmt.Errorf("complete profile find: %w", err)
	}
	if !u.EmailVerified {
		return "", nil, ErrEmailUnverified
	}
	if u.PasswordHash == "" {
		return "", nil, ErrIncompleteSignup
	}

	if other, err := s.users.FindByUsername(ctx, username); err == nil && other.ID != u.ID {
		return "", nil, ErrUsernameTaken
	} else if err != nil && !errors.Is(err, userrepo.ErrNotFound) {
		return "", nil, fmt.Errorf("complete profile username check: %w", err)
	}

	u.Username = username
	u.Country = country
	if err := s.users.Update(ctx, u); err != nil {
		if errors.Is(err, userrepo.ErrDuplicate) {
			return "", nil, ErrUsernameTaken
		}
		return "", nil, fmt.Errorf("complete profile update: %w", err)
	}

	_ = s.sessions.Delete(ctx, signupToken)

	sessionToken, err := randomToken()
	if err != nil {
		return "", nil, fmt.Errorf("complete profile session token: %w", err)
	}
	if err := s.sessions.Create(ctx, sessionToken, sessionrepo.TokenData{
		UserID: u.ID,
		Email:  u.Email,
		Kind:   sessionrepo.KindSession,
	}, 0); err != nil {
		return "", nil, fmt.Errorf("complete profile store session: %w", err)
	}
	return sessionToken, usersvc.ToProfile(u), nil
}

func (s *service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, ErrInvalidEmail
	}
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("login find: %w", err)
	}
	if !u.EmailVerified {
		return nil, ErrEmailUnverified
	}
	if u.PasswordHash == "" {
		// OTP-only progress is not durable — they must restart signup.
		return nil, ErrIncompleteSignup
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrUnauthorized
	}

	if !u.ProfileComplete() {
		signupToken, err := s.issueSignupToken(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("login reissue signup token: %w", err)
		}
		return &LoginResult{
			NeedsProfile: true,
			SignupToken:  signupToken,
			Profile:      usersvc.ToProfile(u),
		}, nil
	}

	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("login token: %w", err)
	}
	if err := s.sessions.Create(ctx, token, sessionrepo.TokenData{
		UserID: u.ID,
		Email:  u.Email,
		Kind:   sessionrepo.KindSession,
	}, 0); err != nil {
		return nil, fmt.Errorf("login store session: %w", err)
	}
	return &LoginResult{
		SessionToken: token,
		Profile:      usersvc.ToProfile(u),
	}, nil
}

func (s *service) SignupStatus(ctx context.Context, signupToken string) (*SignupStatus, error) {
	data, err := s.requireSignup(ctx, signupToken)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, data.UserID)
	if err != nil {
		return nil, fmt.Errorf("signup status find: %w", err)
	}
	return &SignupStatus{
		Email:           u.Email,
		PasswordSet:     u.PasswordHash != "",
		ProfileComplete: u.ProfileComplete(),
	}, nil
}

func (s *service) issueSignupToken(ctx context.Context, u *userrepo.User) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.sessions.Create(ctx, token, sessionrepo.TokenData{
		UserID: u.ID,
		Email:  u.Email,
		Kind:   sessionrepo.KindSignup,
	}, s.cfg.SignupTokenTTL); err != nil {
		return "", fmt.Errorf("store signup token: %w", err)
	}
	return token, nil
}

func (s *service) Logout(ctx context.Context, sessionToken string) error {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return ErrUnauthorized
	}
	data, err := s.sessions.Get(ctx, sessionToken)
	if err != nil {
		if errors.Is(err, sessionrepo.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("logout get: %w", err)
	}
	if data.Kind != sessionrepo.KindSession {
		return ErrUnauthorized
	}
	if err := s.sessions.Delete(ctx, sessionToken); err != nil {
		return fmt.Errorf("logout delete: %w", err)
	}
	return nil
}

func (s *service) StartPasswordReset(ctx context.Context, email string) (bool, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return false, ErrInvalidEmail
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("password reset find user: %w", err)
	}
	// Only fully registered accounts may reset.
	if !u.EmailVerified || u.PasswordHash == "" || !u.ProfileComplete() {
		return false, nil
	}

	code, err := randomDigits(6)
	if err != nil {
		return false, fmt.Errorf("password reset code: %w", err)
	}
	codeHash := hashCode(code)
	expires := time.Now().UTC().Add(s.cfg.VerificationCodeTTL)
	if err := s.codes.Upsert(ctx, email, codeHash, expires); err != nil {
		return false, fmt.Errorf("password reset store code: %w", err)
	}

	body := fmt.Sprintf(
		"Your Meetopoly password reset code is: %s\n\nIt expires in %d minutes.\n",
		code,
		int(s.cfg.VerificationCodeTTL.Minutes()),
	)
	if err := s.mailer.Send(ctx, mailsvc.Message{
		To:      email,
		Subject: "Meetopoly password reset",
		Body:    body,
	}); err != nil {
		return false, fmt.Errorf("password reset mail: %w", err)
	}
	return true, nil
}

func (s *service) VerifyPasswordReset(ctx context.Context, email, code string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", ErrInvalidEmail
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 || !allDigits(code) {
		return "", ErrInvalidCode
	}

	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return "", ErrInvalidCode
		}
		return "", fmt.Errorf("password reset verify find user: %w", err)
	}
	if !u.EmailVerified || u.PasswordHash == "" || !u.ProfileComplete() {
		return "", ErrInvalidCode
	}

	rec, err := s.codes.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, verificationrepo.ErrNotFound) {
			return "", ErrInvalidCode
		}
		return "", fmt.Errorf("password reset verify find code: %w", err)
	}
	if rec.Attempts >= maxVerifyAttempts {
		return "", ErrTooManyAttempts
	}
	if time.Now().UTC().After(rec.ExpiresAt) {
		return "", ErrInvalidCode
	}
	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(hashCode(code))) != 1 {
		_ = s.codes.IncrementAttempts(ctx, email)
		return "", ErrInvalidCode
	}

	if err := s.codes.DeleteByEmail(ctx, email); err != nil {
		return "", fmt.Errorf("password reset verify delete code: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("password reset token: %w", err)
	}
	if err := s.sessions.Create(ctx, token, sessionrepo.TokenData{
		UserID: u.ID,
		Email:  u.Email,
		Kind:   sessionrepo.KindPasswordReset,
	}, s.cfg.SignupTokenTTL); err != nil {
		return "", fmt.Errorf("password reset store token: %w", err)
	}
	return token, nil
}

func (s *service) ResetPassword(ctx context.Context, resetToken, password string) error {
	data, err := s.requirePasswordReset(ctx, resetToken)
	if err != nil {
		return err
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("reset password hash: %w", err)
	}
	u, err := s.users.FindByID(ctx, data.UserID)
	if err != nil {
		return fmt.Errorf("reset password find user: %w", err)
	}
	if !u.ProfileComplete() || u.PasswordHash == "" {
		return ErrIncompleteSignup
	}
	u.PasswordHash = string(hash)
	if err := s.users.Update(ctx, u); err != nil {
		return fmt.Errorf("reset password update: %w", err)
	}
	_ = s.sessions.Delete(ctx, resetToken)
	if err := s.sessions.DeleteAllForUser(ctx, u.ID); err != nil {
		return fmt.Errorf("reset password revoke sessions: %w", err)
	}
	return nil
}

func (s *service) requirePasswordReset(ctx context.Context, token string) (*sessionrepo.TokenData, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthorized
	}
	data, err := s.sessions.Get(ctx, token)
	if err != nil {
		if errors.Is(err, sessionrepo.ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("password reset token: %w", err)
	}
	if data.Kind != sessionrepo.KindPasswordReset {
		return nil, ErrUnauthorized
	}
	return data, nil
}

func (s *service) ResolveSession(ctx context.Context, token string) (string, error) {
	data, err := s.sessions.Get(ctx, strings.TrimSpace(token))
	if err != nil {
		if errors.Is(err, sessionrepo.ErrNotFound) {
			return "", ErrUnauthorized
		}
		return "", fmt.Errorf("resolve session: %w", err)
	}
	if data.Kind != sessionrepo.KindSession {
		return "", ErrUnauthorized
	}
	return data.UserID, nil
}

func (s *service) ResolveSignup(ctx context.Context, token string) (string, string, error) {
	data, err := s.requireSignup(ctx, token)
	if err != nil {
		return "", "", err
	}
	return data.UserID, data.Email, nil
}

func (s *service) requireSignup(ctx context.Context, token string) (*sessionrepo.TokenData, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrUnauthorized
	}
	data, err := s.sessions.Get(ctx, token)
	if err != nil {
		if errors.Is(err, sessionrepo.ErrNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("signup token: %w", err)
	}
	if data.Kind != sessionrepo.KindSignup {
		return nil, ErrUnauthorized
	}
	return data, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "", ErrInvalidEmail
	}
	addr, err := stdmail.ParseAddress(email)
	if err != nil {
		return "", ErrInvalidEmail
	}
	if strings.Contains(addr.Address, " ") {
		return "", ErrInvalidEmail
	}
	return addr.Address, nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 128 {
		return ErrWeakPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrWeakPassword
	}
	return nil
}

func randomDigits(n int) (string, error) {
	const digits = "0123456789"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = digits[int(buf[i])%10]
	}
	return string(out), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
