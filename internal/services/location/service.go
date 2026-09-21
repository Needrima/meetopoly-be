package location

import (
	"context"
	"errors"
	"fmt"
	"strings"

	locationrepo "meetopoly-be/internal/repository/location"
)

type service struct {
	repo locationrepo.Repository
}

// New builds a location Service.
func New(repo locationrepo.Repository) Service {
	return &service{repo: repo}
}

func (s *service) ListWorlds(ctx context.Context) ([]locationrepo.WorldSummary, error) {
	worlds, err := s.repo.ListWorlds(ctx)
	if err != nil {
		return nil, fmt.Errorf("list worlds: %w", err)
	}
	return worlds, nil
}

func (s *service) ListByWorldID(ctx context.Context, worldID string) ([]locationrepo.Location, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, fmt.Errorf("%w: worldId required", ErrInvalidArgument)
	}
	locs, err := s.repo.ListByWorldID(ctx, worldID)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	return locs, nil
}

func (s *service) GetByID(ctx context.Context, id string) (*locationrepo.Location, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: id required", ErrInvalidArgument)
	}
	loc, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, locationrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get location: %w", err)
	}
	return loc, nil
}

func (s *service) GetByWorldAndSlug(ctx context.Context, worldID, slug string) (*locationrepo.Location, error) {
	worldID = strings.TrimSpace(worldID)
	slug = strings.TrimSpace(slug)
	if worldID == "" || slug == "" {
		return nil, fmt.Errorf("%w: worldId and slug required", ErrInvalidArgument)
	}
	loc, err := s.repo.FindByWorldAndSlug(ctx, worldID, slug)
	if err != nil {
		if errors.Is(err, locationrepo.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get location by slug: %w", err)
	}
	return loc, nil
}

// Sentinel errors for the HTTP adapter.
var (
	ErrNotFound        = errors.New("location not found")
	ErrInvalidArgument = errors.New("invalid argument")
)
