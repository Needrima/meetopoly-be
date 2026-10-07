package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	locationrepo "meetopoly-be/internal/repository/location"
)

// RedisCache is a Redis-backed ContentCache for worlds/locations.
type RedisCache struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewRedisCache wraps a Redis client. ttl is the key safety TTL (default 26h).
func NewRedisCache(client *goredis.Client, ttl time.Duration) *RedisCache {
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &RedisCache{client: client, ttl: ttl}
}

func (c *RedisCache) GetWorlds(ctx context.Context) ([]locationrepo.WorldSummary, bool, error) {
	var out []locationrepo.WorldSummary
	hit, err := c.getJSON(ctx, cacheKeyWorlds, &out)
	return out, hit, err
}

func (c *RedisCache) SetWorlds(ctx context.Context, worlds []locationrepo.WorldSummary) error {
	if worlds == nil {
		worlds = []locationrepo.WorldSummary{}
	}
	return c.setJSON(ctx, cacheKeyWorlds, worlds)
}

func (c *RedisCache) GetLocations(ctx context.Context, worldID string) ([]locationrepo.Location, bool, error) {
	var out []locationrepo.Location
	hit, err := c.getJSON(ctx, fmt.Sprintf(cacheKeyLocationsFmt, worldID), &out)
	return out, hit, err
}

func (c *RedisCache) SetLocations(ctx context.Context, worldID string, locs []locationrepo.Location) error {
	if locs == nil {
		locs = []locationrepo.Location{}
	}
	return c.setJSON(ctx, fmt.Sprintf(cacheKeyLocationsFmt, worldID), locs)
}

func (c *RedisCache) GetByID(ctx context.Context, id string) (*locationrepo.Location, bool, error) {
	var out locationrepo.Location
	hit, err := c.getJSON(ctx, fmt.Sprintf(cacheKeyLocIDFmt, id), &out)
	if !hit || err != nil {
		return nil, hit, err
	}
	return &out, true, nil
}

func (c *RedisCache) SetByID(ctx context.Context, loc *locationrepo.Location) error {
	if loc == nil {
		return nil
	}
	return c.setJSON(ctx, fmt.Sprintf(cacheKeyLocIDFmt, loc.ID), loc)
}

func (c *RedisCache) GetBySlug(ctx context.Context, worldID, slug string) (*locationrepo.Location, bool, error) {
	var out locationrepo.Location
	hit, err := c.getJSON(ctx, fmt.Sprintf(cacheKeyLocSlugFmt, worldID, slug), &out)
	if !hit || err != nil {
		return nil, hit, err
	}
	return &out, true, nil
}

func (c *RedisCache) SetBySlug(ctx context.Context, loc *locationrepo.Location) error {
	if loc == nil {
		return nil
	}
	return c.setJSON(ctx, fmt.Sprintf(cacheKeyLocSlugFmt, loc.WorldID, loc.Slug), loc)
}

func (c *RedisCache) getJSON(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("location cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return false, fmt.Errorf("location cache unmarshal %s: %w", key, err)
	}
	return true, nil
}

func (c *RedisCache) setJSON(ctx context.Context, key string, val any) error {
	payload, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("location cache marshal %s: %w", key, err)
	}
	if err := c.client.Set(ctx, key, payload, c.ttl).Err(); err != nil {
		return fmt.Errorf("location cache set %s: %w", key, err)
	}
	return nil
}
