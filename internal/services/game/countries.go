package game

import "context"

// CountryLookup resolves ISO 3166-1 alpha-2 codes for game HUD (Phase 9.0a).
// Optional — when nil, PlayerView.Country stays empty.
type CountryLookup interface {
	CountryForUser(ctx context.Context, userID string) string
}
