package user

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"

	userrepo "meetopoly-be/internal/repository/user"
	"meetopoly-be/internal/services/storage"
)

var (
	// ErrInvalidUsername is returned for bad usernames (Phase 19.0).
	ErrInvalidUsername = errors.New("invalid username")
	// ErrUsernameTaken is returned when username is already used.
	ErrUsernameTaken = errors.New("username taken")
	// ErrNotFound is returned when the user does not exist.
	ErrNotFound = errors.New("user not found")
	// ErrStorageNotConfigured is returned when Supabase env is missing.
	ErrStorageNotConfigured = errors.New("storage not configured")
	// ErrInvalidImage is returned for unsupported avatar payloads.
	ErrInvalidImage = errors.New("invalid image, allowed types: jpg, jpeg, png, webp")
	// ErrImageTooLarge is returned when the avatar exceeds MaxAvatarBytes.
	ErrImageTooLarge = errors.New("image too large")
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

// Profile is the public user shape for HTTP.
type Profile struct {
	ID              string
	Email           string
	Username        string
	Country         string
	AvatarURL       string
	EmailVerified   bool
	ProfileComplete bool
}

// Service reads and updates user profiles (Phase 19.0 username + avatar).
type Service interface {
	GetByID(ctx context.Context, id string) (*Profile, error)
	UpdateUsername(ctx context.Context, userID, username string) (*Profile, error)
	UploadAvatar(ctx context.Context, userID, contentType string, r io.Reader, size int64) (*Profile, error)
	DeleteAvatar(ctx context.Context, userID string) (*Profile, error)
}

type service struct {
	users   userrepo.Repository
	objects storage.ObjectStore
}

// New builds a user Service. objects may be nil (avatar ops fail with ErrStorageNotConfigured).
func New(users userrepo.Repository, objects storage.ObjectStore) Service {
	return &service{users: users, objects: objects}
}

func (s *service) GetByID(ctx context.Context, id string) (*Profile, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return ToProfile(u), nil
}

func (s *service) UpdateUsername(ctx context.Context, userID, username string) (*Profile, error) {
	normalized, err := normalizeUsername(username)
	if err != nil {
		return nil, err
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update username find: %w", err)
	}
	if other, err := s.users.FindByUsername(ctx, normalized); err == nil && other.ID != u.ID {
		return nil, ErrUsernameTaken
	} else if err != nil && !errors.Is(err, userrepo.ErrNotFound) {
		return nil, fmt.Errorf("update username check: %w", err)
	}
	u.Username = normalized
	if err := s.users.Update(ctx, u); err != nil {
		if errors.Is(err, userrepo.ErrDuplicate) {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("update username: %w", err)
	}
	return ToProfile(u), nil
}

func (s *service) UploadAvatar(
	ctx context.Context,
	userID, contentType string,
	r io.Reader,
	size int64,
) (*Profile, error) {
	if s.objects == nil || !s.objects.Configured() {
		return nil, ErrStorageNotConfigured
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("upload avatar find: %w", err)
	}
	prevURL := u.AvatarURL
	publicURL, err := s.objects.UploadAvatar(ctx, userID, contentType, r, size)
	if err != nil {
		return nil, mapStorageErr(err)
	}
	u.AvatarURL = publicURL
	if err := s.users.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("upload avatar update: %w", err)
	}
	// Legacy UUID (or other non-stable) keys are not covered by variant cleanup.
	if prevURL != "" && !isStableAvatarURL(prevURL) {
		if err := s.objects.DeleteByPublicURL(ctx, prevURL); err != nil {
			slog.Error("avatar replace cleanup failed", "err", err, "userId", userID, "prevUrl", prevURL)
			return nil, fmt.Errorf("delete previous avatar: %w", err)
		}
	}
	return ToProfile(u), nil
}

func (s *service) DeleteAvatar(ctx context.Context, userID string) (*Profile, error) {
	if s.objects == nil || !s.objects.Configured() {
		return nil, ErrStorageNotConfigured
	}
	u, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("delete avatar find: %w", err)
	}
	prevURL := u.AvatarURL

	// Storage first so a failed delete can be retried while Mongo still has the URL.
	if err := s.objects.DeleteUserAvatarVariants(ctx, userID, ""); err != nil {
		slog.Error("avatar delete variants failed", "err", err, "userId", userID)
		return nil, fmt.Errorf("delete avatar storage: %w", err)
	}
	if prevURL != "" && !isStableAvatarURL(prevURL) {
		if err := s.objects.DeleteByPublicURL(ctx, prevURL); err != nil &&
			!errors.Is(err, storage.ErrNotConfigured) &&
			!errors.Is(err, storage.ErrInvalidObjectURL) {
			slog.Error("avatar delete legacy failed", "err", err, "userId", userID, "prevUrl", prevURL)
			return nil, fmt.Errorf("delete avatar storage: %w", err)
		}
	}

	u.AvatarURL = ""
	if err := s.users.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("delete avatar update: %w", err)
	}
	return ToProfile(u), nil
}

// isStableAvatarURL reports whether url points at `{userId}/avatar.{jpg|png|webp}`.
func isStableAvatarURL(raw string) bool {
	u := stripURLQuery(raw)
	for _, ext := range storage.AvatarExtensions {
		if strings.HasSuffix(u, "/avatar"+ext) {
			return true
		}
	}
	return false
}

func stripURLQuery(u string) string {
	u = strings.TrimSpace(u)
	if before, _, ok := strings.Cut(u, "?"); ok {
		return before
	}
	return u
}

// ToProfile maps a repository user to a public profile.
func ToProfile(u *userrepo.User) *Profile {
	return &Profile{
		ID:              u.ID,
		Email:           u.Email,
		Username:        u.Username,
		Country:         u.Country,
		AvatarURL:       u.AvatarURL,
		EmailVerified:   u.EmailVerified,
		ProfileComplete: u.ProfileComplete(),
	}
}

func normalizeUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if !usernameRE.MatchString(username) {
		return "", ErrInvalidUsername
	}
	return capitalizeUsername(username), nil
}

func capitalizeUsername(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	capNext := true
	for i := range b {
		c := b[i]
		if c == '_' {
			capNext = true
			continue
		}
		if capNext {
			if c >= 'a' && c <= 'z' {
				b[i] = c - 'a' + 'A'
			}
			capNext = false
			continue
		}
		if c >= 'A' && c <= 'Z' {
			b[i] = c - 'A' + 'a'
		}
	}
	return string(b)
}

func mapStorageErr(err error) error {
	switch {
	case errors.Is(err, storage.ErrNotConfigured):
		return ErrStorageNotConfigured
	case errors.Is(err, storage.ErrInvalidImage):
		return ErrInvalidImage
	case errors.Is(err, storage.ErrTooLarge):
		return ErrImageTooLarge
	default:
		return err
	}
}
