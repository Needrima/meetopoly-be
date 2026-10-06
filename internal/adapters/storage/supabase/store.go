package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"meetopoly-be/internal/services/storage"
)

// Config for Supabase Storage HTTP API (service role).
type Config struct {
	URL            string
	ServiceRoleKey string
	Bucket         string
}

// Store implements storage.ObjectStore against Supabase Storage.
type Store struct {
	baseURL    string
	serviceKey string
	bucket     string
	client     *http.Client
}

// New builds a Store. Empty URL/key → Configured() false.
func New(cfg Config) *Store {
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		bucket = "avatars"
	}
	return &Store{
		baseURL:    base,
		serviceKey: strings.TrimSpace(cfg.ServiceRoleKey),
		bucket:     bucket,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Store) Configured() bool {
	return s != nil && s.baseURL != "" && s.serviceKey != "" && s.bucket != ""
}

func (s *Store) UploadAvatar(
	ctx context.Context,
	userID, contentType string,
	r io.Reader,
	size int64,
) (string, error) {
	if !s.Configured() {
		return "", storage.ErrNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", storage.ErrInvalidImage
	}
	ext, err := extForContentType(contentType)
	if err != nil {
		return "", err
	}
	if size < 0 {
		return "", storage.ErrInvalidImage
	}
	if size > storage.MaxAvatarBytes {
		return "", storage.ErrTooLarge
	}

	limited := io.LimitReader(r, storage.MaxAvatarBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read avatar: %w", err)
	}
	if len(body) == 0 {
		return "", storage.ErrInvalidImage
	}
	if int64(len(body)) > storage.MaxAvatarBytes {
		return "", storage.ErrTooLarge
	}

	// Stable path so replace upserts in place instead of orphaning UUID files.
	objectPath := path.Join(userID, "avatar"+ext)
	endpoint := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, objectPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("apikey", s.serviceKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")
	req.ContentLength = int64(len(body))

	res, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("supabase upload: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("supabase upload status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Drop other extensions so jpg→png (etc.) does not leave orphans.
	if err := s.DeleteUserAvatarVariants(ctx, userID, ext); err != nil {
		return "", fmt.Errorf("cleanup avatar variants: %w", err)
	}

	// Cache-bust query so clients refresh after upsert to the same object key.
	publicURL := fmt.Sprintf(
		"%s/storage/v1/object/public/%s/%s?v=%d",
		s.baseURL,
		s.bucket,
		objectPath,
		time.Now().Unix(),
	)
	return publicURL, nil
}

func (s *Store) DeleteByPublicURL(ctx context.Context, publicURL string) error {
	if !s.Configured() {
		return storage.ErrNotConfigured
	}
	objectPath, ok := s.objectPathFromPublicURL(publicURL)
	if !ok || objectPath == "" {
		return fmt.Errorf("%w: %q", storage.ErrInvalidObjectURL, publicURL)
	}
	return s.deleteObjects(ctx, []string{objectPath})
}

func (s *Store) DeleteUserAvatarVariants(ctx context.Context, userID, keepExt string) error {
	if !s.Configured() {
		return storage.ErrNotConfigured
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return storage.ErrInvalidImage
	}
	keepExt = strings.ToLower(strings.TrimSpace(keepExt))
	paths := make([]string, 0, len(storage.AvatarExtensions))
	for _, ext := range storage.AvatarExtensions {
		if keepExt != "" && ext == keepExt {
			continue
		}
		paths = append(paths, path.Join(userID, "avatar"+ext))
	}
	return s.deleteObjects(ctx, paths)
}

// deleteObjects calls Supabase multi-delete. Body must be {"prefixes":[...]} (not a bare array).
func (s *Store) deleteObjects(ctx context.Context, objectPaths []string) error {
	if len(objectPaths) == 0 {
		return nil
	}
	endpoint := fmt.Sprintf("%s/storage/v1/object/%s", s.baseURL, s.bucket)
	payload, err := json.Marshal(map[string]any{"prefixes": objectPaths})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("apikey", s.serviceKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("supabase delete: %w", err)
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("supabase delete status %d: %s", res.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func (s *Store) objectPathFromPublicURL(publicURL string) (string, bool) {
	publicURL = strings.TrimSpace(publicURL)
	if publicURL == "" {
		return "", false
	}
	if i := strings.Index(publicURL, "?"); i >= 0 {
		publicURL = publicURL[:i]
	}
	prefix := fmt.Sprintf("%s/storage/v1/object/public/%s/", s.baseURL, s.bucket)
	if !strings.HasPrefix(publicURL, prefix) {
		return "", false
	}
	objectPath := strings.TrimPrefix(publicURL, prefix)
	objectPath = strings.Trim(objectPath, "/")
	if objectPath == "" {
		return "", false
	}
	return objectPath, true
}

func extForContentType(ct string) (string, error) {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg", "image/jpg":
		return ".jpg", nil
	case "image/png":
		return ".png", nil
	case "image/webp":
		return ".webp", nil
	default:
		return "", storage.ErrInvalidImage
	}
}
