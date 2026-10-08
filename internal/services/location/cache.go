package location

import (
	"context"
	"time"

	locationrepo "meetopoly-be/internal/repository/location"
)

const (
	cacheKeyPrefix       = "cache:v1:"
	cacheKeyWorlds       = cacheKeyPrefix + "worlds"
	cacheKeyLocationsFmt = cacheKeyPrefix + "locations:%s"
	cacheKeyLocIDFmt     = cacheKeyPrefix + "location:id:%s"
	cacheKeyLocSlugFmt   = cacheKeyPrefix + "location:slug:%s:%s"

	defaultCacheTTL      = 26 * time.Hour
	defaultRefreshEvery  = 24 * time.Hour
	defaultRefreshBootIn = 5 * time.Second
)

// Cache stores warm worlds/locations payloads (Phase 21.0).
type Cache interface {
	GetWorlds(ctx context.Context) ([]locationrepo.WorldSummary, bool, error)
	SetWorlds(ctx context.Context, worlds []locationrepo.WorldSummary) error
	GetLocations(ctx context.Context, worldID string) ([]locationrepo.Location, bool, error)
	SetLocations(ctx context.Context, worldID string, locs []locationrepo.Location) error
	GetByID(ctx context.Context, id string) (*locationrepo.Location, bool, error)
	SetByID(ctx context.Context, loc *locationrepo.Location) error
	GetBySlug(ctx context.Context, worldID, slug string) (*locationrepo.Location, bool, error)
	SetBySlug(ctx context.Context, loc *locationrepo.Location) error
}

// CacheConfig tunes Redis TTL and refresh interval.
type CacheConfig struct {
	TTL           time.Duration
	RefreshEvery  time.Duration
	RefreshBootIn time.Duration
}

// StaticContentCacheMaxAgeSec is HTTP Cache-Control max-age for worlds/locations (Phase 23.5).
// Matches defaultCacheTTL and mobile WORLDS_AND_LOCATIONS_STALE_MS.
func StaticContentCacheMaxAgeSec() int {
	return int(defaultCacheTTL / time.Second)
}

func (c CacheConfig) withDefaults() CacheConfig {
	if c.TTL <= 0 {
		c.TTL = defaultCacheTTL
	}
	if c.RefreshEvery <= 0 {
		c.RefreshEvery = defaultRefreshEvery
	}
	if c.RefreshBootIn <= 0 {
		c.RefreshBootIn = defaultRefreshBootIn
	}
	return c
}
