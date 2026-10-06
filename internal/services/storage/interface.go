package storage

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrNotConfigured is returned when Supabase env is missing.
	ErrNotConfigured = errors.New("storage not configured")
	// ErrInvalidImage is returned for unsupported types or empty bodies.
	ErrInvalidImage = errors.New("invalid image, allowed types: jpg, jpeg, png, webp")
	// ErrTooLarge is returned when the upload exceeds MaxAvatarBytes.
	ErrTooLarge = errors.New("image too large")
)

// MaxAvatarBytes is the upload size cap (1 MiB).
const MaxAvatarBytes = 1 << 20

// ObjectStore uploads and deletes avatar objects (Phase 19.0).
type ObjectStore interface {
	// Configured reports whether uploads can succeed.
	Configured() bool
	// UploadAvatar stores bytes for userID and returns the public URL.
	UploadAvatar(ctx context.Context, userID, contentType string, r io.Reader, size int64) (publicURL string, err error)
	// DeleteByPublicURL removes an object previously returned by UploadAvatar.
	DeleteByPublicURL(ctx context.Context, publicURL string) error
}
