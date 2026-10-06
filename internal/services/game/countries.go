package game

import "context"

// CountryLookup resolves ISO 3166-1 alpha-2 codes for game HUD (Phase 9.0a).
// Optional — when nil, PlayerView.Country stays empty.
type CountryLookup interface {
	CountryForUser(ctx context.Context, userID string) string
}

// AvatarLookup resolves profile photos and live usernames for game HUD (Phase 19.0).
// Optional — when nil, PlayerView.AvatarURL stays empty and Username stays snapshotted.
type AvatarLookup interface {
	AvatarURLForUser(ctx context.Context, userID string) string
	UsernameForUser(ctx context.Context, userID string) string
}
