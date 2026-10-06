package supabase

import (
	"errors"
	"testing"

	"meetopoly-be/internal/services/storage"
)

func TestObjectPathFromPublicURL(t *testing.T) {
	s := &Store{
		baseURL: "https://kdvmvpewlqckfcsopkbr.supabase.co",
		bucket:  "meetopoly",
	}

	t.Run("stable path with cache bust", func(t *testing.T) {
		path, ok := s.objectPathFromPublicURL(
			"https://kdvmvpewlqckfcsopkbr.supabase.co/storage/v1/object/public/meetopoly/uid1/avatar.jpg?v=1710000000",
		)
		if !ok || path != "uid1/avatar.jpg" {
			t.Fatalf("got %q ok=%v", path, ok)
		}
	})

	t.Run("legacy uuid path", func(t *testing.T) {
		path, ok := s.objectPathFromPublicURL(
			"https://kdvmvpewlqckfcsopkbr.supabase.co/storage/v1/object/public/meetopoly/uid1/deadbeef-cafe.jpg",
		)
		if !ok || path != "uid1/deadbeef-cafe.jpg" {
			t.Fatalf("got %q ok=%v", path, ok)
		}
	})

	t.Run("wrong bucket", func(t *testing.T) {
		_, ok := s.objectPathFromPublicURL(
			"https://kdvmvpewlqckfcsopkbr.supabase.co/storage/v1/object/public/other/uid1/avatar.jpg",
		)
		if ok {
			t.Fatal("expected false")
		}
	})
}

func TestDeleteByPublicURLRejectsUnknownURL(t *testing.T) {
	s := &Store{
		baseURL:    "https://example.supabase.co",
		serviceKey: "secret",
		bucket:     "meetopoly",
	}
	err := s.DeleteByPublicURL(t.Context(), "https://cdn.example/not-ours.jpg")
	if !errors.Is(err, storage.ErrInvalidObjectURL) {
		t.Fatalf("got %v", err)
	}
}
