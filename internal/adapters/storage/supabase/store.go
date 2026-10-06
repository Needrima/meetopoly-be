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

	"github.com/google/uuid"

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

	objectPath := path.Join(userID, uuid.NewString()+ext)
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

	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.baseURL, s.bucket, objectPath)
	return publicURL, nil
}

func (s *Store) DeleteByPublicURL(ctx context.Context, publicURL string) error {
	if !s.Configured() {
		return storage.ErrNotConfigured
	}
	objectPath, ok := s.objectPathFromPublicURL(publicURL)
	if !ok || objectPath == "" {
		return nil
	}

	endpoint := fmt.Sprintf("%s/storage/v1/object/%s", s.baseURL, s.bucket)
	payload, err := json.Marshal([]string{objectPath})
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
	prefix := fmt.Sprintf("%s/storage/v1/object/public/%s/", s.baseURL, s.bucket)
	if !strings.HasPrefix(publicURL, prefix) {
		return "", false
	}
	return strings.TrimPrefix(publicURL, prefix), true
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
