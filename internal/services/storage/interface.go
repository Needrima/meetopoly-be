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
	// ErrInvalidObjectURL is returned when a public URL is not in this store's bucket.
	ErrInvalidObjectURL = errors.New("avatar url is not in this storage bucket")
)

// MaxAvatarBytes is the upload size cap (1 MiB).
const MaxAvatarBytes = 1 << 20

// AvatarExtensions are the stable avatar object suffixes under `{userID}/avatar{ext}`.
var AvatarExtensions = []string{".jpg", ".png", ".webp"}

// ObjectStore uploads and deletes avatar objects (Phase 19.0).
type ObjectStore interface {
	Configured() bool
	// UploadAvatar stores bytes for userID and returns the public URL.
	// Implementations should upsert a stable key and remove other avatar.* variants.
	UploadAvatar(ctx context.Context, userID, contentType string, r io.Reader, size int64) (publicURL string, err error)
	// DeleteByPublicURL removes an object previously returned by UploadAvatar.
	DeleteByPublicURL(ctx context.Context, publicURL string) error
	// DeleteUserAvatarVariants removes `{userID}/avatar.{jpg,png,webp}` except keepExt
	// (e.g. ".jpg"). Empty keepExt deletes all variants.
	DeleteUserAvatarVariants(ctx context.Context, userID, keepExt string) error
}
