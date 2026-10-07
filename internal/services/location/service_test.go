package location

import (
	"context"
	"errors"
	"sync"
	"testing"

	locationrepo "meetopoly-be/internal/repository/location"
)

type memRepo struct {
	mu         sync.Mutex
	worlds     []locationrepo.WorldSummary
	byWorld    map[string][]locationrepo.Location
	byID       map[string]locationrepo.Location
	listCalls  int
	worldCalls int
	idCalls    int
	slugCalls  int
}

func newMemRepo() *memRepo {
	africa := []locationrepo.Location{
		{ID: "loc-1", WorldID: "africa-1", Slug: "lagos", Name: "Lagos", BoardIndex: 0},
		{ID: "loc-2", WorldID: "africa-1", Slug: "accra", Name: "Accra", BoardIndex: 1},
	}
	m := &memRepo{
		worlds:  []locationrepo.WorldSummary{{WorldID: "africa-1", Count: 2}},
		byWorld: map[string][]locationrepo.Location{"africa-1": africa},
		byID:    map[string]locationrepo.Location{},
	}
	for _, loc := range africa {
		m.byID[loc.ID] = loc
	}
	return m
}

func (m *memRepo) EnsureIndexes(context.Context) error { return nil }

func (m *memRepo) ListByWorldID(_ context.Context, worldID string) ([]locationrepo.Location, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listCalls++
	locs, ok := m.byWorld[worldID]
	if !ok {
		return nil, nil
	}
	out := append([]locationrepo.Location(nil), locs...)
	return out, nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (*locationrepo.Location, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idCalls++
	loc, ok := m.byID[id]
	if !ok {
		return nil, locationrepo.ErrNotFound
	}
	cp := loc
	return &cp, nil
}

func (m *memRepo) FindByWorldAndSlug(_ context.Context, worldID, slug string) (*locationrepo.Location, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slugCalls++
	for _, loc := range m.byWorld[worldID] {
		if loc.Slug == slug {
			cp := loc
			return &cp, nil
		}
	}
	return nil, locationrepo.ErrNotFound
}

func (m *memRepo) ListWorlds(context.Context) ([]locationrepo.WorldSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.worldCalls++
	return append([]locationrepo.WorldSummary(nil), m.worlds...), nil
}

func (m *memRepo) ReplaceAll(context.Context, []locationrepo.Location) (int, error) {
	return 0, errors.New("not implemented")
}

type memCache struct {
	mu        sync.Mutex
	worlds    []locationrepo.WorldSummary
	hasWorlds bool
	lists     map[string][]locationrepo.Location
	byID      map[string]locationrepo.Location
	bySlug    map[string]locationrepo.Location
}

func newMemCache() *memCache {
	return &memCache{
		lists:  map[string][]locationrepo.Location{},
		byID:   map[string]locationrepo.Location{},
		bySlug: map[string]locationrepo.Location{},
	}
}

func (c *memCache) GetWorlds(context.Context) ([]locationrepo.WorldSummary, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasWorlds {
		return nil, false, nil
	}
	return append([]locationrepo.WorldSummary(nil), c.worlds...), true, nil
}

func (c *memCache) SetWorlds(_ context.Context, worlds []locationrepo.WorldSummary) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.worlds = append([]locationrepo.WorldSummary(nil), worlds...)
	c.hasWorlds = true
	return nil
}

func (c *memCache) GetLocations(_ context.Context, worldID string) ([]locationrepo.Location, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	locs, ok := c.lists[worldID]
	if !ok {
		return nil, false, nil
	}
	return append([]locationrepo.Location(nil), locs...), true, nil
}

func (c *memCache) SetLocations(_ context.Context, worldID string, locs []locationrepo.Location) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lists[worldID] = append([]locationrepo.Location(nil), locs...)
	return nil
}

func (c *memCache) GetByID(_ context.Context, id string) (*locationrepo.Location, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	loc, ok := c.byID[id]
	if !ok {
		return nil, false, nil
	}
	cp := loc
	return &cp, true, nil
}

func (c *memCache) SetByID(_ context.Context, loc *locationrepo.Location) error {
	if loc == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byID[loc.ID] = *loc
	return nil
}

func (c *memCache) GetBySlug(_ context.Context, worldID, slug string) (*locationrepo.Location, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	loc, ok := c.bySlug[worldID+"\x00"+slug]
	if !ok {
		return nil, false, nil
	}
	cp := loc
	return &cp, true, nil
}

func (c *memCache) SetBySlug(_ context.Context, loc *locationrepo.Location) error {
	if loc == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bySlug[loc.WorldID+"\x00"+loc.Slug] = *loc
	return nil
}

func TestListWorldsMissThenHit(t *testing.T) {
	repo := newMemRepo()
	cache := newMemCache()
	svc := New(repo, cache, CacheConfig{})

	if _, err := svc.ListWorlds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.worldCalls != 1 {
		t.Fatalf("worldCalls=%d want 1", repo.worldCalls)
	}
	if _, err := svc.ListWorlds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.worldCalls != 1 {
		t.Fatalf("second list should hit cache, worldCalls=%d", repo.worldCalls)
	}
}

func TestListByWorldIDMissThenHit(t *testing.T) {
	repo := newMemRepo()
	cache := newMemCache()
	svc := New(repo, cache, CacheConfig{})

	locs, err := svc.ListByWorldID(context.Background(), "africa-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 2 {
		t.Fatalf("len=%d", len(locs))
	}
	if repo.listCalls != 1 {
		t.Fatalf("listCalls=%d want 1", repo.listCalls)
	}
	if _, err := svc.ListByWorldID(context.Background(), "africa-1"); err != nil {
		t.Fatal(err)
	}
	if repo.listCalls != 1 {
		t.Fatalf("second list should hit cache, listCalls=%d", repo.listCalls)
	}
	// Warm filled by-id / by-slug.
	if _, err := svc.GetByID(context.Background(), "loc-1"); err != nil {
		t.Fatal(err)
	}
	if repo.idCalls != 0 {
		t.Fatalf("GetByID should hit cache after list warm, idCalls=%d", repo.idCalls)
	}
}

func TestRefreshCacheWarmsAll(t *testing.T) {
	repo := newMemRepo()
	cache := newMemCache()
	svc := New(repo, cache, CacheConfig{})

	if err := svc.RefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
	repo.worldCalls = 0
	repo.listCalls = 0
	repo.idCalls = 0

	if _, err := svc.ListWorlds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListByWorldID(context.Background(), "africa-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetByWorldAndSlug(context.Background(), "africa-1", "lagos"); err != nil {
		t.Fatal(err)
	}
	if repo.worldCalls != 0 || repo.listCalls != 0 || repo.slugCalls != 0 {
		t.Fatalf("expected full cache hit after refresh (world=%d list=%d slug=%d)",
			repo.worldCalls, repo.listCalls, repo.slugCalls)
	}
}

func TestNilCacheStillWorks(t *testing.T) {
	repo := newMemRepo()
	svc := New(repo, nil, CacheConfig{})
	if _, err := svc.ListWorlds(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := svc.RefreshCache(context.Background()); err != nil {
		t.Fatal(err)
	}
}
