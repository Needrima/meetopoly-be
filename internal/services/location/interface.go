package location

import (
	"context"

	locationrepo "meetopoly-be/internal/repository/location"
)

// Service reads World board locations.
type Service interface {
	ListWorlds(ctx context.Context) ([]locationrepo.WorldSummary, error)
	ListByWorldID(ctx context.Context, worldID string) ([]locationrepo.Location, error)
	GetByID(ctx context.Context, id string) (*locationrepo.Location, error)
	GetByWorldAndSlug(ctx context.Context, worldID, slug string) (*locationrepo.Location, error)
	// RefreshCache warms Redis from Mongo (Phase 21.0). No-op when cache is nil.
	RefreshCache(ctx context.Context) error
	// StartCacheRefresher runs boot + periodic RefreshCache until ctx is cancelled.
	StartCacheRefresher(ctx context.Context)
}
