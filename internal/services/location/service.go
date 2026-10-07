package location

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	locationrepo "meetopoly-be/internal/repository/location"
)

type service struct {
	repo  locationrepo.Repository
	cache Cache
	cfg   CacheConfig
}

// New builds a location Service. cache may be nil (Mongo-only).
func New(repo locationrepo.Repository, cache Cache, cfg CacheConfig) Service {
	return &service{
		repo:  repo,
		cache: cache,
		cfg:   cfg.withDefaults(),
	}
}

func (s *service) ListWorlds(ctx context.Context) ([]locationrepo.WorldSummary, error) {
	if s.cache != nil {
		if worlds, hit, err := s.cache.GetWorlds(ctx); err != nil {
			slog.Warn("location cache get worlds failed", "err", err)
		} else if hit {
			return worlds, nil
		}
	}
	worlds, err := s.repo.ListWorlds(ctx)
	if err != nil {
		return nil, fmt.Errorf("list worlds: %w", err)
	}
	if s.cache != nil {
		if setErr := s.cache.SetWorlds(ctx, worlds); setErr != nil {
			slog.Warn("location cache set worlds failed", "err", setErr)
		}
	}
	return worlds, nil
}

func (s *service) ListByWorldID(ctx context.Context, worldID string) ([]locationrepo.Location, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, fmt.Errorf("%w: worldId required", ErrInvalidArgument)
	}
	if s.cache != nil {
		if locs, hit, err := s.cache.GetLocations(ctx, worldID); err != nil {
			slog.Warn("location cache get list failed", "worldId", worldID, "err", err)
		} else if hit {
			return locs, nil
		}
	}
	locs, err := s.repo.ListByWorldID(ctx, worldID)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	if s.cache != nil {
		if setErr := s.cache.SetLocations(ctx, worldID, locs); setErr != nil {
			slog.Warn("location cache set list failed", "worldId", worldID, "err", setErr)
		}
		s.warmLocationEntries(ctx, locs)
	}
	return locs, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*locationrepo.Location, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: id required", ErrInvalidArgument)
	}
	if s.cache != nil {
		if loc, hit, err := s.cache.GetByID(ctx, id); err != nil {
			slog.Warn("location cache get by id failed", "id", id, "err", err)
		} else if hit {
			return loc, nil
		}
	}
	loc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, locationrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get location: %w", err)
	}
	if s.cache != nil {
		if setErr := s.cache.SetByID(ctx, loc); setErr != nil {
			slog.Warn("location cache set by id failed", "id", id, "err", setErr)
		}
		if setErr := s.cache.SetBySlug(ctx, loc); setErr != nil {
			slog.Warn("location cache set by slug failed", "id", id, "err", setErr)
		}
	}
	return loc, nil
}

func (s *service) GetByWorldAndSlug(ctx context.Context, worldID, slug string) (*locationrepo.Location, error) {
	worldID = strings.TrimSpace(worldID)
	slug = strings.TrimSpace(slug)
	if worldID == "" || slug == "" {
		return nil, fmt.Errorf("%w: worldId and slug required", ErrInvalidArgument)
	}
	if s.cache != nil {
		if loc, hit, err := s.cache.GetBySlug(ctx, worldID, slug); err != nil {
			slog.Warn("location cache get by slug failed", "worldId", worldID, "slug", slug, "err", err)
		} else if hit {
			return loc, nil
		}
	}
	loc, err := s.repo.FindByWorldAndSlug(ctx, worldID, slug)
	if err != nil {
		if errors.Is(err, locationrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get location by slug: %w", err)
	}
	if s.cache != nil {
		if setErr := s.cache.SetBySlug(ctx, loc); setErr != nil {
			slog.Warn("location cache set by slug failed", "err", setErr)
		}
		if setErr := s.cache.SetByID(ctx, loc); setErr != nil {
			slog.Warn("location cache set by id failed", "err", setErr)
		}
	}
	return loc, nil
}

// RefreshCache loads all worlds/locations from Mongo into Redis (Phase 21.0).
func (s *service) RefreshCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	worlds, err := s.repo.ListWorlds(ctx)
	if err != nil {
		return fmt.Errorf("refresh worlds: %w", err)
	}
	if err := s.cache.SetWorlds(ctx, worlds); err != nil {
		return fmt.Errorf("refresh set worlds: %w", err)
	}
	for _, w := range worlds {
		locs, err := s.repo.ListByWorldID(ctx, w.WorldID)
		if err != nil {
			return fmt.Errorf("refresh list %s: %w", w.WorldID, err)
		}
		if err := s.cache.SetLocations(ctx, w.WorldID, locs); err != nil {
			return fmt.Errorf("refresh set list %s: %w", w.WorldID, err)
		}
		s.warmLocationEntries(ctx, locs)
	}
	slog.Info("location cache refreshed", "worlds", len(worlds))
	return nil
}

// StartCacheRefresher warms Redis shortly after boot, then every RefreshEvery.
func (s *service) StartCacheRefresher(ctx context.Context) {
	if s.cache == nil {
		return
	}
	bootIn := s.cfg.RefreshBootIn
	every := s.cfg.RefreshEvery
	go func() {
		timer := time.NewTimer(bootIn)
		defer timer.Stop()
		ticker := time.NewTicker(every)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if err := s.RefreshCache(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("location cache refresh failed", "err", err)
				}
			case <-ticker.C:
				if err := s.RefreshCache(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("location cache refresh failed", "err", err)
				}
			}
		}
	}()
}

func (s *service) warmLocationEntries(ctx context.Context, locs []locationrepo.Location) {
	for i := range locs {
		loc := locs[i]
		if err := s.cache.SetByID(ctx, &loc); err != nil {
			slog.Warn("location cache warm id failed", "id", loc.ID, "err", err)
		}
		if err := s.cache.SetBySlug(ctx, &loc); err != nil {
			slog.Warn("location cache warm slug failed", "slug", loc.Slug, "err", err)
		}
	}
}

// Sentinel errors for the HTTP adapter.
var (
	ErrNotFound        = errors.New("location not found")
	ErrInvalidArgument = errors.New("invalid argument")
)
