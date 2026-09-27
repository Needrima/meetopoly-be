package game

import (
	"context"
	"sync"

	gamerepo "meetopoly-be/internal/repository/game"
	locationrepo "meetopoly-be/internal/repository/location"
)

// Space is a board tile used for buy / rent / tax / build rules (Phase 6.4+ / 11.1).
type Space struct {
	BoardIndex        int
	Slug              string
	Name              string
	Kind              string
	Price             int
	Rents             []int
	UtilityMultiplier []int
	TaxAmount         int
	SpecialType       string
	ColorGroup        string
	HouseCost         int // MeetCoin per house/hotel step; 0 = not buildable
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

// NewLocationSpaceCatalog adapts the location repository for game buy/rent rules.
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
		sp := Space{
			BoardIndex:        loc.BoardIndex,
			Slug:              loc.Slug,
			Name:              loc.Name,
			Kind:              loc.Kind,
			Price:             loc.Price,
			Rents:             append([]int(nil), loc.Rents...),
			UtilityMultiplier: append([]int(nil), loc.UtilityMultiplier...),
		}
		if loc.ColorGroup != nil {
			sp.ColorGroup = *loc.ColorGroup
		}
		if loc.SpecialType != nil {
			sp.SpecialType = *loc.SpecialType
		}
		if loc.TaxAmount != nil {
			sp.TaxAmount = *loc.TaxAmount
		}
		if loc.HouseCost != nil {
			sp.HouseCost = *loc.HouseCost
		}
		out = append(out, sp)
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

// housesOnDeed returns clamped house count (0–5; 5 = hotel) for a deed.
func housesOnDeed(deeds []gamerepo.Deed, boardIndex int) int {
	for _, d := range deeds {
		if d.BoardIndex == boardIndex {
			h := d.Houses
			if h < 0 {
				return 0
			}
			if h > 5 {
				return 5
			}
			return h
		}
	}
	return 0
}

func countOwnedKind(spaces []Space, deeds []gamerepo.Deed, ownerUserID, kind string) int {
	n := 0
	owned := make(map[int]bool, len(deeds))
	for _, d := range deeds {
		if d.OwnerUserID == ownerUserID {
			owned[d.BoardIndex] = true
		}
	}
	for _, sp := range spaces {
		if sp.Kind == kind && owned[sp.BoardIndex] {
			n++
		}
	}
	return n
}

// ownsFullColorGroup reports whether ownerUserID owns every property in colorGroup.
func ownsFullColorGroup(spaces []Space, deeds []gamerepo.Deed, ownerUserID, colorGroup string) bool {
	if colorGroup == "" {
		return false
	}
	owned := make(map[int]bool, len(deeds))
	for _, d := range deeds {
		if d.OwnerUserID == ownerUserID {
			owned[d.BoardIndex] = true
		}
	}
	inGroup := 0
	for _, sp := range spaces {
		if sp.Kind != "property" || sp.ColorGroup != colorGroup {
			continue
		}
		inGroup++
		if !owned[sp.BoardIndex] {
			return false
		}
	}
	// Classic sets are 2–3; avoid false monopoly when catalog is incomplete in tests.
	return inGroup >= 2
}

// minHousesInColorGroup is the lowest house count among deeds the owner holds in the set.
func minHousesInColorGroup(spaces []Space, deeds []gamerepo.Deed, ownerUserID, colorGroup string) int {
	byIndex := make(map[int]gamerepo.Deed, len(deeds))
	for _, d := range deeds {
		if d.OwnerUserID == ownerUserID {
			byIndex[d.BoardIndex] = d
		}
	}
	minH := -1
	for _, sp := range spaces {
		if sp.Kind != "property" || sp.ColorGroup != colorGroup {
			continue
		}
		d, ok := byIndex[sp.BoardIndex]
		if !ok {
			continue
		}
		h := d.Houses
		if h < 0 {
			h = 0
		}
		if h > 5 {
			h = 5
		}
		if minH < 0 || h < minH {
			minH = h
		}
	}
	if minH < 0 {
		return 0
	}
	return minH
}

// colorGroupHasMortgage is true when any deed in the color group is mortgaged.
func colorGroupHasMortgage(spaces []Space, deeds []gamerepo.Deed, colorGroup string) bool {
	inGroup := make(map[int]bool)
	for _, sp := range spaces {
		if sp.Kind == "property" && sp.ColorGroup == colorGroup {
			inGroup[sp.BoardIndex] = true
		}
	}
	for _, d := range deeds {
		if inGroup[d.BoardIndex] && d.Mortgaged {
			return true
		}
	}
	return false
}

// rentDueForLanding computes MeetCoin owed when payer lands on boardIndex (0 = nothing).
func rentDueForLanding(
	spaces []Space,
	deeds []gamerepo.Deed,
	boardIndex int,
	payerUserID string,
	diceTotal int,
) (amount int, toUserID string, kind string, spaceName string) {
	sp := spaceAt(spaces, boardIndex)
	if sp == nil {
		return 0, "", "", ""
	}
	spaceName = sp.Name

	if sp.SpecialType == "tax" && sp.TaxAmount > 0 {
		return sp.TaxAmount, "", "tax", spaceName
	}

	if !isBuyableKind(sp.Kind) {
		return 0, "", "", spaceName
	}
	ownerID := ownerOf(deeds, boardIndex)
	if ownerID == "" || ownerID == payerUserID {
		return 0, "", "", spaceName
	}

	switch sp.Kind {
	case "property":
		if len(sp.Rents) == 0 || sp.Rents[0] <= 0 {
			return 0, ownerID, "rent", spaceName
		}
		houses := housesOnDeed(deeds, boardIndex)
		monopoly := ownsFullColorGroup(spaces, deeds, ownerID, sp.ColorGroup)
		// No monopoly → site rent only (ignore houses; impossible under classic rules).
		if !monopoly {
			return sp.Rents[0], ownerID, "rent", spaceName
		}
		// Undeveloped monopoly → 2× site.
		if houses == 0 {
			return sp.Rents[0] * 2, ownerID, "rent", spaceName
		}
		// Monopoly + houses ≥ 1 → Rents[houses] (5 = hotel).
		idx := houses
		if idx >= len(sp.Rents) {
			idx = len(sp.Rents) - 1
		}
		return sp.Rents[idx], ownerID, "rent", spaceName
	case "railroad":
		n := countOwnedKind(spaces, deeds, ownerID, "railroad")
		table := sp.Rents
		if len(table) == 0 {
			table = []int{25, 50, 100, 200}
		}
		idx := n - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(table) {
			idx = len(table) - 1
		}
		return table[idx], ownerID, "rent", spaceName
	case "utility":
		n := countOwnedKind(spaces, deeds, ownerID, "utility")
		mult := 4
		if n >= 2 {
			mult = 10
		}
		if len(sp.UtilityMultiplier) >= 1 && n < 2 {
			mult = sp.UtilityMultiplier[0]
		}
		if len(sp.UtilityMultiplier) >= 2 && n >= 2 {
			mult = sp.UtilityMultiplier[1]
		}
		if diceTotal < 2 {
			diceTotal = 2
		}
		return mult * diceTotal, ownerID, "rent", spaceName
	default:
		return 0, "", "", spaceName
	}
}
