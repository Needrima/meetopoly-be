package game

import (
	"context"
	"sync"

	gamerepo "meetopoly-be/internal/repository/game"
	locationrepo "meetopoly-be/internal/repository/location"
)

// Space is a board tile used for buy rules (Phase 6.4).
type Space struct {
	BoardIndex int
	Slug       string
	Name       string
	Kind       string
	Price      int
}

// SpaceCatalog loads world board tiles for the game service.
type SpaceCatalog interface {
	ListSpaces(ctx context.Context, worldID string) ([]Space, error)
}

type locationSpaceCatalog struct {
	repo  locationrepo.Repository
	mu    sync.Mutex
	cache map[string][]Space
}

// NewLocationSpaceCatalog adapts the location repository for game buy rules.
func NewLocationSpaceCatalog(repo locationrepo.Repository) SpaceCatalog {
	return &locationSpaceCatalog{
		repo:  repo,
		cache: make(map[string][]Space),
	}
}

func (c *locationSpaceCatalog) ListSpaces(ctx context.Context, worldID string) ([]Space, error) {
	c.mu.Lock()
	if cached, ok := c.cache[worldID]; ok {
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	locs, err := c.repo.ListByWorldID(ctx, worldID)
	if err != nil {
		return nil, err
	}
	out := make([]Space, 0, len(locs))
	for _, loc := range locs {
		out = append(out, Space{
			BoardIndex: loc.BoardIndex,
			Slug:       loc.Slug,
			Name:       loc.Name,
			Kind:       loc.Kind,
			Price:      loc.Price,
		})
	}
	c.mu.Lock()
	c.cache[worldID] = out
	c.mu.Unlock()
	return out, nil
}

func isBuyableKind(kind string) bool {
	switch kind {
	case "property", "railroad", "utility":
		return true
	default:
		return false
	}
}

func spaceAt(spaces []Space, boardIndex int) *Space {
	for i := range spaces {
		if spaces[i].BoardIndex == boardIndex {
			return &spaces[i]
		}
	}
	return nil
}

func ownerOf(deeds []gamerepo.Deed, boardIndex int) string {
	for _, d := range deeds {
		if d.BoardIndex == boardIndex {
			return d.OwnerUserID
		}
	}
	return ""
}
